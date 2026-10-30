package gqlclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"graphql-api-tests-go/gqlclient"
)

type countryData struct {
	Country *struct {
		Name string `json:"name"`
	} `json:"country"`
}

func serve(t *testing.T, status int, body string, inspect func(*http.Request)) *gqlclient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if inspect != nil {
			inspect(r)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return gqlclient.New(srv.URL, gqlclient.WithHeader("Authorization", "Bearer x"))
}

func TestExecute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		status     int
		body       string
		wantErr    bool
		wantHTTP   bool
		wantGQLErr string
		wantName   string
	}{
		{name: "success", status: 200, body: `{"data":{"country":{"name":"Ukraine"}}}`, wantName: "Ukraine"},
		{name: "null data", status: 200, body: `{"data":{"country":null}}`},
		{name: "graphql errors with 200", status: 200, body: `{"errors":[{"message":"boom","extensions":{"code":"X"}}]}`, wantGQLErr: "boom"},
		{name: "graphql errors with 400", status: 400, body: `{"errors":[{"message":"bad"}]}`, wantGQLErr: "bad"},
		{name: "http 500 without gql body", status: 500, body: `oops`, wantErr: true, wantHTTP: true},
		{name: "invalid json with 200", status: 200, body: `<html>`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := serve(t, tc.status, tc.body, nil)
			resp, err := gqlclient.Execute[countryData](context.Background(), c, gqlclient.Request{Query: "{x}"})
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if _, isHTTP := gqlclient.IsHTTPError(err); isHTTP != tc.wantHTTP {
					t.Fatalf("IsHTTPError = %v, want %v (err: %v)", isHTTP, tc.wantHTTP, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantGQLErr != "" {
				if len(resp.Errors) != 1 || resp.Errors[0].Message != tc.wantGQLErr {
					t.Fatalf("errors = %+v, want message %q", resp.Errors, tc.wantGQLErr)
				}
				if resp.Err() == nil {
					t.Fatal("Err() should be non-nil")
				}
				return
			}
			if resp.Err() != nil {
				t.Fatalf("unexpected graphql errors: %v", resp.Err())
			}
			got := ""
			if resp.Data.Country != nil {
				got = resp.Data.Country.Name
			}
			if got != tc.wantName {
				t.Fatalf("name = %q, want %q", got, tc.wantName)
			}
		})
	}
}

func TestExecuteSendsRequestAndHeaders(t *testing.T) {
	t.Parallel()
	var gotReq gqlclient.Request
	var gotAuth, gotCT string
	c := serve(t, 200, `{"data":{}}`, func(r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
	})
	_, err := gqlclient.Execute[map[string]any](context.Background(), c, gqlclient.Request{
		Query:     "query Q($a: ID!){x(a:$a)}",
		Variables: map[string]any{"a": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer x" || gotCT != "application/json" {
		t.Fatalf("headers: auth=%q content-type=%q", gotAuth, gotCT)
	}
	if gotReq.Variables["a"] != "1" || gotReq.Query == "" {
		t.Fatalf("unexpected request body: %+v", gotReq)
	}
}

func TestExecuteTimeout(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	c := gqlclient.New(srv.URL, gqlclient.WithTimeout(50*time.Millisecond))
	if _, err := gqlclient.Execute[map[string]any](context.Background(), c, gqlclient.Request{Query: "{x}"}); err == nil {
		t.Fatal("expected timeout error")
	}
}
