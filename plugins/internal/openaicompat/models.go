// Package openaicompat provides utilities for interacting with
// OpenAI-compatible APIs.
package openaicompat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Datum contains the well-known fields of a model object returned by GET
// /v1/models. Callers needing provider-specific extra fields should embed
// Datum in their own type and use the resulting type with [GetModels].
type Datum struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// embeddedDatum is a private marker method enforcing that types embedding
// Datum satisfy [EmbedsDatum].
func (Datum) embeddedDatum() {}

// EmbedsDatum is a marker interface for types that embed [Datum].
type EmbedsDatum interface {
	embeddedDatum()
}

// DataResponse represents the object returned by GET /v1/models: a "data"
// array of datums of type T. Callers needing provider-specific extra top-level
// fields (e.g. llama.cpp's "models") should embed DataResponse in their own
// type and pass a pointer to that type as [GetModels]' out argument.
//
// See the example in [GetModels] for a demonstration of how to add custom
// fields.
type DataResponse[T EmbedsDatum] struct {
	Data []T `json:"data"`
}

type SimpleModelsResponse = DataResponse[Datum]

// GetModels calls GET <baseURL>/v1/models with an optional bearer token, and
// decodes the response body into out. It is recommended that out should be a
// pointer to a type that embeds DataResponse. For the simplest cases, use a
// pointer to a [SimpleModelsResponse] struct as the type of out. For more
// complex cases, use [DataResponse] with a custom struct, or even define a
// custom wrapper struct and embed [DataResponse] in it.
func GetModels(ctx context.Context, client *http.Client, baseURL, bearerToken string, out any) error {
	addr := strings.TrimRight(baseURL, "/") + "/v1/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr, nil)
	if err != nil {
		return fmt.Errorf("building /v1/models request: %w", err)
	}

	if strings.TrimSpace(bearerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("calling /v1/models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("/v1/models returned %s", resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding /v1/models response: %w", err)
	}

	return nil
}
