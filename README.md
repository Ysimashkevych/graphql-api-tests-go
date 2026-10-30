# graphql-api-tests-go

A small, idiomatic Go framework for testing GraphQL APIs, demonstrated against the public
[Countries API](https://countries.trevorblades.com/). It is a clean, public re-creation of the approach
I built for GraphQL API testing at work (that code is private; nothing here is copied from it).

## What it shows

- A reusable typed client (`gqlclient`) that separates transport errors, HTTP errors and the GraphQL `errors` array.
- Queries kept as `.graphql` files and embedded with `go:embed` (`queries`).
- Test helpers (`testkit`): GraphQL error assertions, introspection-based schema contract checks, JSON response-shape checks.
- Table-driven tests with parallel subtests, positive and negative cases.
- Offline tests (default) separated from live-network tests by the `live` build tag.

## Layout

```
gqlclient/   Client, Request, Response[T], Error/Errors, HTTPError, options (headers, timeout, HTTP client)
queries/     *.graphql documents + Load(name) via go:embed
testkit/     Helpers: Run, RequireNoErrors, RequireErrorContaining, FieldTypes/AssertFieldTypes, AssertJSONShape
tests/       Live tests against the real API (//go:build live)
```

### Error model

`gqlclient.Execute[T]` returns:

| Situation | Result |
|---|---|
| Network failure, timeout, undecodable body | `error` |
| Non-2xx status without a GraphQL body | `*HTTPError` (check with `IsHTTPError`) |
| GraphQL `errors` array (any status) | `Response` with `Errors` set, `error == nil`; `resp.Err()` converts it |

Keeping GraphQL errors in the response (not the Go error) lets negative tests assert on them directly.

## Running

Requires Go (see `go.mod`).

```
make test        # offline: client unit tests (httptest) + helper tests, no network
make test-live   # live tests against the public API (-tags live)
make lint        # go vet + gofmt check (+ golangci-lint if installed)
```

Use `GRAPHQL_API_URL` to point the live tests at another endpoint. CI runs offline tests on pushes to `main` and on pull requests. The live suite depends on a third-party service, so it is
advisory on push/PR and blocking on the weekly schedule and manual runs, where a failure signals upstream drift.

## Why standard library only

Standard library only, no testify. The helpers here are small, a failing assertion
reports exactly what is needed, and zero dependencies means no `go.sum`, no supply-chain surface and an easy read for reviewers.
The trade-off is a little more boilerplate than `assert`/`require`; `testkit` absorbs most of it.

## Notes

- Live assertions (e.g. capitals, schema types) reflect the Countries API at the time of writing; if the upstream data
  or schema changes, those tests will fail by design, which is the point of a contract check.

## Related projects

- [playwright-ts-framework](https://github.com/Ysimashkevych/playwright-ts-framework): Playwright + TypeScript UI and API framework.
- [ai-test-triage](https://github.com/Ysimashkevych/ai-test-triage): LLM-assisted triage of Playwright failures.
- [qa-claude-skills](https://github.com/Ysimashkevych/qa-claude-skills): Claude Code skills for QA, including tests from GraphQL schemas.

## License

MIT, see [LICENSE](LICENSE).
