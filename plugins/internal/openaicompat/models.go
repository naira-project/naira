// Package openaicompat provides utilities for interacting with
// OpenAI-compatible APIs.
package openaicompat

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/naira-project/naira/plugins/pkg/httpjson"
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

	err := httpjson.Get(ctx, client, addr, out,
		httpjson.WithAuthorizationBearer(bearerToken))
	if err != nil {
		return fmt.Errorf("calling /v1/models: %w", err)
	}

	return nil
}
