// Package openaiutil provides a small client for the parts of the OpenAI API
// that Naira plugins need, so that every plugin talking to an OpenAI-compatible
// endpoint shares one implementation.
package openaiutil

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

// embeddedDatum is a private marker method enforcing that types passed as
// FetchModels' D type parameter embed Datum.
func (Datum) embeddedDatum() {}

type EmbedsDatum interface {
	embeddedDatum()
}

// ModelsResponse mirrors the root object returned by GET /v1/models: a "data"
// array of datums of type T. Callers needing provider-specific extra
// top-level fields (e.g. llama.cpp's "models") embed ModelsResponse in their
// own type and pass that type as FetchModels' T type parameter.
type ModelsResponse[T EmbedsDatum] struct {
	Data []T `json:"data"`
}

// embeddedModelsResponse is a private marker method enforcing that types
// passed as FetchModels' T type parameter embed ModelsResponse[D].
func (ModelsResponse[T]) embeddedModelsResponse() {}

type EmbedsModelsResponse[T EmbedsDatum] interface {
	embeddedModelsResponse()
}

// FetchModels calls GET <baseURL>/v1/models with an optional bearer token,
// and decodes the response body into T. T must embed ModelsResponse[D] (see
// EmbedsModelsResponse), which lets callers capture provider-specific extra
// fields by embedding Datum/ModelsResponse in their own types instead of
// losing them to the well-known fields alone.
func FetchModels[T EmbedsModelsResponse[D], D EmbedsDatum](ctx context.Context, client *http.Client, baseURL, bearerToken string) (T, error) {
	var zero T

	addr := strings.TrimRight(baseURL, "/") + "/v1/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr, nil)
	if err != nil {
		return zero, fmt.Errorf("preparing %q request: %w", addr, err)
	}
	if strings.TrimSpace(bearerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return zero, fmt.Errorf("executing %q request: %w", addr, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return zero, fmt.Errorf("%q returned %s: %w", addr, resp.Status, ErrUnauthorized)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return zero, fmt.Errorf("%q returned %s", addr, resp.Status)
	}

	var payload T
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return zero, fmt.Errorf("parsing %q response: %w", addr, err)
	}

	return payload, nil
}

// ModelIDs reduces a datum list to its IDs, preserving order.
func ModelIDs(models []Datum) []string {
	ids := make([]string, 0, len(models))
	for _, m := range models {
		ids = append(ids, m.ID)
	}
	return ids
}
