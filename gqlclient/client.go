// Package gqlclient is a small GraphQL-over-HTTP client with typed responses.
//
// It separates three failure classes:
//   - transport/decoding problems: Execute returns a non-nil error;
//   - non-2xx HTTP status without a GraphQL body: Execute returns *HTTPError;
//   - GraphQL-level "errors" array: Execute returns the Response with Errors
//     populated and a nil error, so negative tests can assert on them. Call
//     Response.Err to turn them into an error.
package gqlclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// DefaultTimeout is applied when no custom HTTP client is supplied.
const DefaultTimeout = 15 * time.Second

// Request is a GraphQL request payload.
type Request struct {
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables,omitempty"`
	OperationName string         `json:"operationName,omitempty"`
}

// Location is a position in the query document.
type Location struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// Error is one entry of the GraphQL "errors" array.
type Error struct {
	Message    string         `json:"message"`
	Locations  []Location     `json:"locations,omitempty"`
	Path       []any          `json:"path,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

// Error implements the error interface.
func (e Error) Error() string { return e.Message }

// Code returns extensions.code if present.
func (e Error) Code() string {
	s, _ := e.Extensions["code"].(string)
	return s
}

// Errors is a non-empty list of GraphQL errors returned as one error value.
type Errors []Error

// Error implements the error interface.
func (es Errors) Error() string {
	msgs := make([]string, len(es))
	for i, e := range es {
		msgs[i] = e.Message
	}
	return fmt.Sprintf("graphql: %d error(s): %s", len(es), strings.Join(msgs, "; "))
}

// HTTPError is returned for a non-2xx response that carries no GraphQL errors.
type HTTPError struct {
	StatusCode int
	Body       string
}

// Error implements the error interface.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("graphql: unexpected HTTP status %d: %s", e.StatusCode, e.Body)
}

// Response is a decoded GraphQL response; Data is typed by the caller.
type Response[T any] struct {
	Data       T
	Errors     Errors
	StatusCode int
	Duration   time.Duration
	// Raw is the undecoded response body, useful for contract assertions.
	Raw json.RawMessage
}

// Err returns the GraphQL errors as a single error, or nil.
func (r *Response[T]) Err() error {
	if len(r.Errors) == 0 {
		return nil
	}
	return r.Errors
}

// Client sends GraphQL requests to one endpoint.
type Client struct {
	endpoint  string
	http      *http.Client
	headers   http.Header
	userAgent string
	timeout   time.Duration
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient replaces the underlying http.Client (e.g. a custom transport).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithTimeout sets the overall per-request timeout. It is applied to a copy of
// the HTTP client, so a client passed via WithHTTPClient is never modified, and
// the result does not depend on option order.
func WithTimeout(d time.Duration) Option { return func(c *Client) { c.timeout = d } }

// WithHeader sets a header sent with every request (e.g. Authorization).
func WithHeader(key, value string) Option { return func(c *Client) { c.headers.Set(key, value) } }

// WithUserAgent overrides the User-Agent header.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// New creates a Client for endpoint.
func New(endpoint string, opts ...Option) *Client {
	c := &Client{
		endpoint:  endpoint,
		http:      &http.Client{Timeout: DefaultTimeout},
		headers:   http.Header{},
		userAgent: "graphql-api-tests-go",
	}
	for _, o := range opts {
		o(c)
	}
	if c.timeout > 0 {
		hc := *c.http
		hc.Timeout = c.timeout
		c.http = &hc
	}
	return c
}

// Execute sends req and decodes "data" into T. See the package comment for how
// errors are classified.
func Execute[T any](ctx context.Context, c *Client, req Request) (*Response[T], error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("gqlclient: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gqlclient: build request: %w", err)
	}
	for k, vs := range c.headers {
		for _, v := range vs {
			httpReq.Header.Add(k, v)
		}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", c.userAgent)

	start := time.Now()
	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gqlclient: send request: %w", err)
	}
	defer httpResp.Body.Close()
	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("gqlclient: read response: %w", err)
	}

	resp := &Response[T]{
		StatusCode: httpResp.StatusCode,
		Duration:   time.Since(start),
		Raw:        raw,
	}

	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors Errors          `json:"errors"`
	}
	decodeErr := json.Unmarshal(raw, &envelope)

	ok := httpResp.StatusCode >= 200 && httpResp.StatusCode < 300
	if decodeErr != nil || (!ok && len(envelope.Errors) == 0) {
		if !ok {
			return resp, &HTTPError{StatusCode: httpResp.StatusCode, Body: truncate(string(raw), 512)}
		}
		return resp, fmt.Errorf("gqlclient: decode response: %w", decodeErr)
	}

	resp.Errors = envelope.Errors
	if len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, &resp.Data); err != nil {
			return resp, fmt.Errorf("gqlclient: decode data: %w", err)
		}
	}
	return resp, nil
}

// IsHTTPError reports whether err is an *HTTPError and returns it.
func IsHTTPError(err error) (*HTTPError, bool) {
	var he *HTTPError
	if errors.As(err, &he) {
		return he, true
	}
	return nil, false
}

// truncate shortens s to at most n bytes without splitting a UTF-8 sequence.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "..."
}
