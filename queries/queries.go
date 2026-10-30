// Package queries holds the GraphQL documents used by the tests. They live in
// .graphql files (editor/linters friendly) and are compiled into the binary
// with go:embed, so tests do not depend on the working directory.
package queries

import (
	"embed"
	"fmt"
)

//go:embed *.graphql
var files embed.FS

// Load returns the document stored in <name>.graphql. It panics on a missing
// file: that is a programming error and should fail tests loudly.
func Load(name string) string {
	b, err := files.ReadFile(name + ".graphql")
	if err != nil {
		panic(fmt.Sprintf("queries: %v", err))
	}
	return string(b)
}

// Names of the embedded documents.
const (
	CountryByCode    = "country_by_code"
	CountriesByCodes = "countries_by_codes"
	Continents       = "continents"
	InvalidField     = "invalid_field"
	IntrospectType   = "introspect_type"
)
