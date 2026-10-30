// Package testkit contains helpers shared by the API tests: client setup,
// GraphQL error assertions, introspection-based schema checks and JSON shape
// (contract) assertions. Only the standard library is used.
package testkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Ysimashkevych/graphql-api-tests-go/gqlclient"
	"github.com/Ysimashkevych/graphql-api-tests-go/queries"
)

// DefaultEndpoint is the public Countries GraphQL API.
const DefaultEndpoint = "https://countries.trevorblades.com/"

// EndpointEnv overrides the endpoint under test.
const EndpointEnv = "GRAPHQL_API_URL"

// Endpoint returns the endpoint from the environment or the default.
func Endpoint() string {
	if v := os.Getenv(EndpointEnv); v != "" {
		return v
	}
	return DefaultEndpoint
}

// NewClient returns a client for Endpoint().
func NewClient() *gqlclient.Client {
	return gqlclient.New(Endpoint(), gqlclient.WithTimeout(20*time.Second))
}

// Context returns a context bound to the test's lifetime with a deadline.
func Context(t testing.TB) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// Run loads the named embedded query and executes it, failing the test on
// transport/HTTP errors. GraphQL errors are left in the response for the
// caller to assert on.
func Run[T any](t testing.TB, c *gqlclient.Client, query string, vars map[string]any) *gqlclient.Response[T] {
	t.Helper()
	resp, err := gqlclient.Execute[T](Context(t), c, gqlclient.Request{
		Query:     queries.Load(query),
		Variables: vars,
	})
	if err != nil {
		t.Fatalf("executing %q: %v", query, err)
	}
	return resp
}

// RequireNoErrors fails the test if the response has GraphQL errors.
func RequireNoErrors[T any](t testing.TB, resp *gqlclient.Response[T]) {
	t.Helper()
	if err := resp.Err(); err != nil {
		t.Fatalf("unexpected GraphQL errors: %v", err)
	}
}

// RequireErrorContaining fails unless some error message contains substr
// (case-insensitive). It returns the matching error.
func RequireErrorContaining[T any](t testing.TB, resp *gqlclient.Response[T], substr string) gqlclient.Error {
	t.Helper()
	for _, e := range resp.Errors {
		if strings.Contains(strings.ToLower(e.Message), strings.ToLower(substr)) {
			return e
		}
	}
	t.Fatalf("no GraphQL error containing %q; errors: %v", substr, resp.Errors)
	return gqlclient.Error{}
}

// TypeRef mirrors the introspection __Type reference (up to 4 wrappers deep).
type TypeRef struct {
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	OfType *TypeRef `json:"ofType"`
}

// String renders the reference in SDL notation, e.g. "[Language!]!". A
// wrapper nested deeper than the introspection query fetches renders as
// "<truncated>" rather than silently dropping the inner type.
func (r *TypeRef) String() string {
	if r == nil {
		return "<truncated>"
	}
	switch r.Kind {
	case "NON_NULL":
		return r.OfType.String() + "!"
	case "LIST":
		return "[" + r.OfType.String() + "]"
	default:
		return r.Name
	}
}

// IntrospectedType is the result of the introspect_type query.
type IntrospectedType struct {
	Type *struct {
		Name   string `json:"name"`
		Kind   string `json:"kind"`
		Fields []struct {
			Name string  `json:"name"`
			Type TypeRef `json:"type"`
		} `json:"fields"`
	} `json:"__type"`
}

// FieldTypes returns field name -> SDL type for an object type via introspection.
func FieldTypes(t testing.TB, c *gqlclient.Client, typeName string) map[string]string {
	t.Helper()
	resp := Run[IntrospectedType](t, c, queries.IntrospectType, map[string]any{"name": typeName})
	RequireNoErrors(t, resp)
	if resp.Data.Type == nil {
		t.Fatalf("type %q not found in schema", typeName)
	}
	out := make(map[string]string, len(resp.Data.Type.Fields))
	for _, f := range resp.Data.Type.Fields {
		out[f.Name] = f.Type.String()
	}
	return out
}

// AssertFieldTypes checks that every wanted field exists with exactly the
// wanted SDL type. Extra fields in the schema are allowed (additive changes
// are not breaking).
func AssertFieldTypes(t testing.TB, got, want map[string]string) {
	t.Helper()
	names := make([]string, 0, len(want))
	for n := range want {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		g, ok := got[n]
		switch {
		case !ok:
			t.Errorf("field %q missing from schema", n)
		case g != want[n]:
			t.Errorf("field %q has type %s, want %s", n, g, want[n])
		}
	}
}

// AssertJSONShape walks path (dot-separated keys) in a raw response body and
// checks that the object found there has all wanted keys. Use "data.country"
// style paths; list elements are not traversed, so point it at an object.
func AssertJSONShape(t testing.TB, raw json.RawMessage, path string, wantKeys ...string) {
	t.Helper()
	var cur any
	if err := json.Unmarshal(raw, &cur); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if path != "" {
		for _, seg := range strings.Split(path, ".") {
			m, ok := cur.(map[string]any)
			if !ok {
				t.Fatalf("path %q: %s is not an object (got %T)", path, seg, cur)
			}
			cur, ok = m[seg]
			if !ok {
				t.Fatalf("path %q: key %q not found", path, seg)
			}
		}
	}
	obj, ok := cur.(map[string]any)
	if !ok {
		t.Fatalf("path %q: value is %T, want object", path, cur)
	}
	for _, k := range wantKeys {
		if _, ok := obj[k]; !ok {
			t.Errorf("path %q: missing key %q (have %s)", path, k, keys(obj))
		}
	}
}

func keys(m map[string]any) string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return fmt.Sprint(ks)
}
