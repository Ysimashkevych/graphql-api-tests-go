package testkit_test

import (
	"encoding/json"
	"testing"

	"github.com/Ysimashkevych/graphql-api-tests-go/testkit"
)

func TestTypeRefString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		json string
		want string
	}{
		{"scalar", `{"kind":"SCALAR","name":"String"}`, "String"},
		{"non-null", `{"kind":"NON_NULL","ofType":{"kind":"SCALAR","name":"ID"}}`, "ID!"},
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
