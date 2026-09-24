// Package openaiutil provides a small client for the parts of the OpenAI API
// that Naira plugins need, so that every plugin talking to an OpenAI-compatible
// endpoint shares one implementation.
package openaicompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var ErrUnauthorized = errors.New("unauthorized")

// Datum mirrors the well-known fields of a model object returned by GET
// /v1/models. Callers needing provider-specific extra fields embed Datum in
// their own type and pass that type as FetchModels' D type parameter.
type Datum struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// embeddedDatum is a private marker method enforcing that types embedding
// Datum satisfy EmbedsDatum.
func (Datum) embeddedDatum() {}

type EmbedsDatum interface {
	embeddedDatum()
}

// DataResponse mirrors the root object returned by GET /v1/models: a "data"
// array of datums of type T. Callers needing provider-specific extra
// top-level fields (e.g. llama.cpp's "models") embed DataResponse in their
// own type and pass a pointer to that type as FetchModels' out argument.
type DataResponse[T EmbedsDatum] struct {
	Data []T `json:"data"`
}

type SimpleModelsResponse = DataResponse[Datum]

// GetModels calls GET <baseURL>/v1/models with an optional bearer token, and
// decodes the response body into out. out should be a pointer to a type that
// embeds DataResponse. For the simplest cases, use a pointer to a
// SimpleModelsResponse struct as the type of out. For more complex cases, use
// DataResponse with a custom struct, or even define a custom wrapper struct
// and embed DataResponse in it.
func GetModels(ctx context.Context, client *http.Client, baseURL, bearerToken string, out any) error {
	addr := strings.TrimRight(baseURL, "/") + "/v1/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr, nil)
	if err != nil {
		return fmt.Errorf("preparing %q request: %w", addr, err)
	}
	if strings.TrimSpace(bearerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("executing %q request: %w", addr, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%q returned %s: %w", addr, resp.Status, ErrUnauthorized)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return fmt.Errorf("%q returned %s", addr, resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("parsing %q response: %w", addr, err)
	}

	return nil
}
