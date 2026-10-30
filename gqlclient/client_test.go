package gqlclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ysimashkevych/graphql-api-tests-go/gqlclient"
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
		{name: "neither data nor errors", status: 200, body: `{}`, wantErr: true},
		{name: "null data without errors", status: 200, body: `{"data":null}`, wantErr: true},
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
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	t.Cleanup(srv.Close)
	c := gqlclient.New(srv.URL, gqlclient.WithHeader("Authorization", "Bearer x"), gqlclient.WithUserAgent("portfolio-tests"))
	_, err := gqlclient.Execute[map[string]any](context.Background(), c, gqlclient.Request{
		Query:     "query Q($a: ID!){x(a:$a)}",
		Variables: map[string]any{"a": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{
		"Authorization": "Bearer x",
		"Content-Type":  "application/json",
		"Accept":        "application/json",
		"User-Agent":    "portfolio-tests",
	} {
		if got := gotHeaders.Get(k); got != want {
			t.Errorf("header %s = %q, want %q", k, got, want)
		}
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
	start := time.Now()
	_, err := gqlclient.Execute[map[string]any](context.Background(), c, gqlclient.Request{Query: "{x}"})
	var ne net.Error
	if !errors.As(err, &ne) || !ne.Timeout() {
		t.Fatalf("expected a timeout error, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("timeout took %v; the 50ms client timeout was not applied", elapsed)
	}
}

func TestWithTimeoutDoesNotModifySharedClient(t *testing.T) {
	shared := &http.Client{Timeout: time.Minute}
	for _, opts := range [][]gqlclient.Option{
		{gqlclient.WithHTTPClient(shared), gqlclient.WithTimeout(time.Second)},
		{gqlclient.WithTimeout(time.Second), gqlclient.WithHTTPClient(shared)},
	} {
		gqlclient.New("http://example.invalid", opts...)
		if shared.Timeout != time.Minute {
			t.Fatalf("shared client timeout changed to %v", shared.Timeout)
		}
	}
}

func TestErrorFormatting(t *testing.T) {
	t.Parallel()
	e := gqlclient.Error{Message: "boom", Extensions: map[string]any{"code": "BAD_USER_INPUT"}}
	if e.Code() != "BAD_USER_INPUT" {
		t.Errorf("Code() = %q", e.Code())
	}
	if (gqlclient.Error{Message: "x"}).Code() != "" {
		t.Error("Code() without extensions should be empty")
	}
	if got, want := (gqlclient.Errors{e, {Message: "second"}}).Error(), "graphql: 2 error(s): boom; second"; got != want {
		t.Errorf("Errors.Error() = %q, want %q", got, want)
	}
}
