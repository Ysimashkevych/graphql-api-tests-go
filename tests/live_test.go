//go:build live

// Live tests hit the public Countries API over the network. They are excluded
// from a plain `go test ./...`; run them with `go test -tags live ./tests/...`
// (see the Makefile).
package tests

import (
	"testing"

	"github.com/Ysimashkevych/graphql-api-tests-go/gqlclient"
	"github.com/Ysimashkevych/graphql-api-tests-go/queries"
	"github.com/Ysimashkevych/graphql-api-tests-go/testkit"
)

type country struct {
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Capital   *string `json:"capital"`
	Currency  *string `json:"currency"`
	Continent struct {
		Code string `json:"code"`
		Name string `json:"name"`
	} `json:"continent"`
	Languages []struct {
		Code string `json:"code"`
		Name string `json:"name"`
	} `json:"languages"`
}

type countryData struct {
	Country *country `json:"country"`
}

type countriesData struct {
	Countries []struct {
		Code      string `json:"code"`
		Name      string `json:"name"`
		Continent struct {
			Code string `json:"code"`
		} `json:"continent"`
	} `json:"countries"`
}

type continentsData struct {
	Continents []struct {
		Code string `json:"code"`
		Name string `json:"name"`
	} `json:"continents"`
}

func TestCountryByCode(t *testing.T) {
	t.Parallel()
	c := testkit.NewClient()
	tests := []struct {
		code, name, capital, continent string
	}{
		{"UA", "Ukraine", "Kyiv", "EU"},
		{"PL", "Poland", "Warsaw", "EU"},
		{"JP", "Japan", "Tokyo", "AS"},
		{"BR", "Brazil", "Brasília", "SA"},
	}
	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			t.Parallel()
			resp := testkit.Run[countryData](t, c, queries.CountryByCode, map[string]any{"code": tc.code})
			testkit.RequireNoErrors(t, resp)
			got := resp.Data.Country
			if got == nil {
				t.Fatalf("country %s: got null", tc.code)
			}
			if got.Code != tc.code || got.Name != tc.name {
				t.Errorf("got %s/%s, want %s/%s", got.Code, got.Name, tc.code, tc.name)
			}
			if got.Capital == nil || *got.Capital != tc.capital {
				t.Errorf("capital = %v, want %s", got.Capital, tc.capital)
			}
			if got.Continent.Code != tc.continent {
				t.Errorf("continent = %s, want %s", got.Continent.Code, tc.continent)
			}
			if len(got.Languages) == 0 {
				t.Error("expected at least one language")
			}
		})
	}
}

func TestCountryNotFoundIsNullNotError(t *testing.T) {
	t.Parallel()
	resp := testkit.Run[countryData](t, testkit.NewClient(), queries.CountryByCode, map[string]any{"code": "ZZ"})
	testkit.RequireNoErrors(t, resp)
	if resp.Data.Country != nil {
		t.Fatalf("expected null country, got %+v", resp.Data.Country)
	}
}

func TestCountriesFilterByCodes(t *testing.T) {
	t.Parallel()
	resp := testkit.Run[countriesData](t, testkit.NewClient(), queries.CountriesByCodes,
		map[string]any{"codes": []string{"UA", "PL", "DE"}})
	testkit.RequireNoErrors(t, resp)
	got := map[string]bool{}
	for _, c := range resp.Data.Countries {
		got[c.Code] = true
		if c.Continent.Code != "EU" {
			t.Errorf("%s: continent %s, want EU", c.Code, c.Continent.Code)
		}
	}
	for _, want := range []string{"UA", "PL", "DE"} {
		if !got[want] {
			t.Errorf("missing %s in result", want)
		}
	}
	if len(resp.Data.Countries) != 3 {
		t.Errorf("got %d countries, want 3", len(resp.Data.Countries))
	}
}

func TestContinents(t *testing.T) {
	t.Parallel()
	resp := testkit.Run[continentsData](t, testkit.NewClient(), queries.Continents, nil)
	testkit.RequireNoErrors(t, resp)
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	have := map[string]string{}
	for _, c := range resp.Data.Continents {
		have[c.Code] = c.Name
	}
	for code, name := range map[string]string{"AF": "Africa", "EU": "Europe", "OC": "Oceania"} {
		if have[code] != name {
			t.Errorf("continent %s = %q, want %q", code, have[code], name)
		}
	}
}

// Contract: the response keeps the shape consumers depend on.
func TestCountryResponseShape(t *testing.T) {
	t.Parallel()
	resp := testkit.Run[countryData](t, testkit.NewClient(), queries.CountryByCode, map[string]any{"code": "UA"})
	testkit.RequireNoErrors(t, resp)
	testkit.AssertJSONShape(t, resp.Raw, "data.country", "code", "name", "capital", "currency", "continent", "languages")
	testkit.AssertJSONShape(t, resp.Raw, "data.country.continent", "code", "name")
}

// Schema contract via introspection: breaking changes (removed field, changed
// nullability) fail here.
func TestSchemaContract(t *testing.T) {
	t.Parallel()
	c := testkit.NewClient()
	tests := []struct {
		typeName string
		want     map[string]string
	}{
		{"Country", map[string]string{
			"code": "ID!", "name": "String!", "capital": "String",
			"currency": "String", "continent": "Continent!", "languages": "[Language!]!",
		}},
		{"Continent", map[string]string{"code": "ID!", "name": "String!", "countries": "[Country!]!"}},
		{"Language", map[string]string{"code": "ID!", "name": "String!", "native": "String!", "rtl": "Boolean!"}},
	}
	for _, tc := range tests {
		t.Run(tc.typeName, func(t *testing.T) {
			t.Parallel()
			testkit.AssertFieldTypes(t, testkit.FieldTypes(t, c, tc.typeName), tc.want)
		})
	}
}

func TestUnknownTypeIntrospectsToNull(t *testing.T) {
	t.Parallel()
	resp := testkit.Run[testkit.IntrospectedType](t, testkit.NewClient(), queries.IntrospectType,
		map[string]any{"name": "DoesNotExist"})
	testkit.RequireNoErrors(t, resp)
	if resp.Data.Type != nil {
		t.Fatalf("expected null __type, got %+v", resp.Data.Type)
	}
}

// Negative tests: the server must report problems in the "errors" array.
func TestNegative(t *testing.T) {
	t.Parallel()
	c := testkit.NewClient()
	tests := []struct {
		name    string
		query   string
		vars    map[string]any
		wantMsg string
	}{
		{"invalid field", queries.InvalidField, nil, `Cannot query field "nope"`},
		{"missing required variable", queries.CountryByCode, nil, "was not provided"},
		{"variable of wrong type", queries.CountryByCode, map[string]any{"code": map[string]any{"a": 1}}, "ID cannot represent"},
		{"list variable with wrong element type", queries.CountriesByCodes, map[string]any{"codes": []any{1}}, "String cannot represent"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp := testkit.Run[map[string]any](t, c, tc.query, tc.vars)
			testkit.RequireErrorContaining(t, resp, tc.wantMsg)
		})
	}
}

func TestSyntaxErrorReported(t *testing.T) {
	t.Parallel()
	resp, err := gqlclient.Execute[map[string]any](testkit.Context(t), testkit.NewClient(),
		gqlclient.Request{Query: "{ country(code: "})
	if err != nil {
		// Some servers answer 400 with a non-GraphQL body; that is an HTTP error.
		if he, ok := gqlclient.IsHTTPError(err); ok && he.StatusCode >= 400 && he.StatusCode < 500 {
			return
		}
		t.Fatal(err)
	}
	// The wording is server-specific (this API answers with a generic "not a
	// valid GraphQL request" message rather than "Syntax Error"), so assert
	// that a non-empty error is reported, not its text.
	if len(resp.Errors) == 0 || resp.Errors[0].Message == "" {
		t.Fatalf("expected a GraphQL error for a malformed query, got %+v", resp.Errors)
	}
}
