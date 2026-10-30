package testkit_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Ysimashkevych/graphql-api-tests-go/gqlclient"
	"github.com/Ysimashkevych/graphql-api-tests-go/testkit"
)

// fakeTB records failures instead of failing the real test, so the helpers'
// failure paths can be asserted. Fatalf panics with errFatal to stop the
// helper the way runtime.Goexit would.
type fakeTB struct {
	testing.TB
	errors []string
	fatal  bool
}

var errFatal = fmt.Errorf("fatal")

func (f *fakeTB) Helper() {}
func (f *fakeTB) Errorf(format string, args ...any) {
	f.errors = append(f.errors, fmt.Sprintf(format, args...))
}
func (f *fakeTB) Fatalf(format string, args ...any) {
	f.Errorf(format, args...)
	f.fatal = true
	panic(errFatal)
}

// run calls fn with a fakeTB and reports whether it recorded a failure.
func run(t *testing.T, fn func(tb testing.TB)) *fakeTB {
	t.Helper()
	f := &fakeTB{TB: t}
	func() {
		defer func() {
			if r := recover(); r != nil && r != errFatal {
				panic(r)
			}
		}()
		fn(f)
	}()
	return f
}

func TestTypeRefString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		json string
		want string
	}{
		{"scalar", `{"kind":"SCALAR","name":"String"}`, "String"},
		{"non-null", `{"kind":"NON_NULL","ofType":{"kind":"SCALAR","name":"ID"}}`, "ID!"},
		{"truncated wrapper", `{"kind":"NON_NULL"}`, "<truncated>!"},
		{"list of non-null non-null", `{"kind":"NON_NULL","ofType":{"kind":"LIST","ofType":{"kind":"NON_NULL","ofType":{"kind":"OBJECT","name":"Language"}}}}`, "[Language!]!"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var r testkit.TypeRef
			if err := json.Unmarshal([]byte(tc.json), &r); err != nil {
				t.Fatal(err)
			}
			if got := r.String(); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestAssertJSONShape(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"data":{"country":{"name":"Ukraine","code":"UA"}}}`)
	testkit.AssertJSONShape(t, raw, "data.country", "name", "code")
}

func TestAssertJSONShapeFailures(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"data":{"country":{"name":"Ukraine"},"list":[1]}}`)
	tests := []struct {
		name, path string
		keys       []string
		wantFatal  bool
	}{
		{"missing key", "data.country", []string{"name", "code"}, false},
		{"missing path segment", "data.nope", []string{"name"}, true},
		{"path through a list", "data.list.x", nil, true},
		{"value is not an object", "data.list", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := run(t, func(tb testing.TB) { testkit.AssertJSONShape(tb, raw, tc.path, tc.keys...) })
			if len(f.errors) == 0 || f.fatal != tc.wantFatal {
				t.Fatalf("errors=%v fatal=%v, want a failure (fatal=%v)", f.errors, f.fatal, tc.wantFatal)
			}
		})
	}
}

func TestAssertFieldTypes(t *testing.T) {
	t.Parallel()
	got := map[string]string{"code": "ID!", "name": "String!", "extra": "Int"}
	if f := run(t, func(tb testing.TB) {
		testkit.AssertFieldTypes(tb, got, map[string]string{"code": "ID!", "name": "String!"})
	}); len(f.errors) != 0 {
		t.Fatalf("matching schema reported errors: %v", f.errors)
	}
	f := run(t, func(tb testing.TB) {
		testkit.AssertFieldTypes(tb, got, map[string]string{"code": "ID", "missing": "String"})
	})
	if len(f.errors) != 2 {
		t.Fatalf("want 2 errors (changed nullability, missing field), got %v", f.errors)
	}
}

func TestGraphQLErrorHelpers(t *testing.T) {
	t.Parallel()
	ok := &gqlclient.Response[any]{}
	bad := &gqlclient.Response[any]{Errors: gqlclient.Errors{{Message: "Cannot query field \"nope\""}}}

	if f := run(t, func(tb testing.TB) { testkit.RequireNoErrors(tb, ok) }); len(f.errors) != 0 {
		t.Fatalf("RequireNoErrors failed on a clean response: %v", f.errors)
	}
	if f := run(t, func(tb testing.TB) { testkit.RequireNoErrors(tb, bad) }); !f.fatal {
		t.Fatal("RequireNoErrors passed on a response with errors")
	}
	if f := run(t, func(tb testing.TB) { testkit.RequireErrorContaining(tb, bad, "cannot query") }); len(f.errors) != 0 {
		t.Fatalf("RequireErrorContaining missed a case-insensitive match: %v", f.errors)
	}
	if f := run(t, func(tb testing.TB) { testkit.RequireErrorContaining(tb, ok, "anything") }); !f.fatal {
		t.Fatal("RequireErrorContaining passed with no errors")
	}
}
