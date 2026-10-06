// Package httpjson provides helpers for calling JSON HTTP APIs.
package httpjson

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// IsStatusAcceptError returns true if err is a [*StatusAcceptError] and its
// StatusCode is one of the codes (or, if no codes are provided, any
// [*StatusAcceptError]).
func IsStatusAcceptError(err error, codes ...int) bool {
	var e *StatusAcceptError
	if !errors.As(err, &e) {
		return false
	}
	if len(codes) == 0 {
		return true
	}
	for _, code := range codes {
		if e.StatusCode == code {
			return true
		}
	}
	return false
}

// StatusAcceptError wraps an unexpected HTTP status response.
type StatusAcceptError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *StatusAcceptError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("unexpected HTTP status %s", e.Status)
	}
	return fmt.Sprintf("unexpected HTTP status %s: %s", e.Status, e.Body)
}

// BuildURL appends query params to base, overwriting any pre-existing query
// param of the same name, and returns the updated URL.
func BuildURL(base string, params url.Values) (*url.URL, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("parsing %q: %w", base, err)
	}

	query := parsed.Query()
	for key, values := range params {
		query[key] = values
	}
	parsed.RawQuery = query.Encode()

	return parsed, nil
}

type options struct {
	headers         http.Header
	acceptStatus    func(code int) bool
	maxResponseBody int64
}

// Option can tweak the configuration of a [Do] call. See the With* functions
// and types for supported features.
type Option interface {
	apply(*options)
}

type optionFunc func(*options)

func (f optionFunc) apply(o *options) { f(o) }

// WithAuthorizationBearer sets an "Authorization: Bearer " + token header if
// the token is not blank. (Whitespace-only counts as blank.)
func WithAuthorizationBearer(token string) Option {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return WithHeaders(map[string]string{
		"Authorization": "Bearer " + token,
	})
}

// WithHeaders sets arbitrary request headers, overwriting any preexisting
// value for the same key.
type WithHeaders map[string]string

func (h WithHeaders) apply(o *options) {
	for key, value := range h {
		o.headers.Set(key, value)
	}
}

// WithAcceptStatus overrides the default "any 2xx" status check with a
// custom predicate.
func WithAcceptStatus(accept func(code int) bool) Option {
	return optionFunc(func(o *options) {
		o.acceptStatus = accept
	})
}

// WithMaxResponseBytes caps how much of the response body is read for
// JSON-decoding it. Zero (the default) means unlimited.
func WithMaxResponseBytes(n int64) Option {
	return optionFunc(func(o *options) {
		o.maxResponseBody = n
	})
}

func is2xx(code int) bool {
	return code >= 200 && code < 300
}

// Do builds an HTTP request for method and url, optionally JSON-encoding
// payload as the request body (skipped if payload is nil), sends it via
// client, checks the response status (by default any 2xx; see
// [WithAcceptStatus]), and JSON-decodes the response body into out (skipped if
// out is nil).
//
// On a rejected status, Do returns a [*StatusAcceptError] wrapping a short
// snippet of the response body.
func Do(ctx context.Context, client *http.Client, method, url string, payload, out any, opts ...Option) error {
	o := options{
		headers:      make(http.Header),
		acceptStatus: is2xx,
	}
	for _, opt := range opts {
		if opt != nil {
			opt.apply(&o)
		}
	}

	var reqBody io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
		reqBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, values := range o.headers {
		for _, value := range values {
			req.Header.Set(key, value)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if !o.acceptStatus(resp.StatusCode) {
		const maxSnippetBytes = 512
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxSnippetBytes))
		return &StatusAcceptError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       strings.TrimSpace(string(snippet)),
		}
	}

	if out == nil {
		return nil
	}

	var respBody io.Reader = resp.Body
	if o.maxResponseBody > 0 {
		respBody = io.LimitReader(resp.Body, o.maxResponseBody)
	}

	if err := json.NewDecoder(respBody).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	return nil
}

// Get is a convenience wrapper around [Do] for a GET request with no request
// body.
func Get(ctx context.Context, client *http.Client, url string, out any, opts ...Option) error {
	return Do(ctx, client, http.MethodGet, url, nil, out, opts...)
}

// Post is a convenience wrapper around [Do] for a POST request, optionally
// JSON-encoding payload as the request body (skipped if payload is nil).
func Post(ctx context.Context, client *http.Client, url string, payload, out any, opts ...Option) error {
	return Do(ctx, client, http.MethodPost, url, payload, out, opts...)
}
