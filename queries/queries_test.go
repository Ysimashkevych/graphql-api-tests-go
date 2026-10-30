package queries_test

import (
	"strings"
	"testing"

	"github.com/Ysimashkevych/graphql-api-tests-go/queries"
)

// Every exported name must match an embedded file; a typo would otherwise
// only surface as a panic in the live suite.
func TestEveryQueryLoads(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		queries.CountryByCode, queries.CountriesByCodes, queries.Continents,
		queries.InvalidField, queries.IntrospectType,
	} {
		if strings.TrimSpace(queries.Load(name)) == "" {
			t.Errorf("%s.graphql is empty", name)
		}
	}
}
