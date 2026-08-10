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

// Model mirrors the model object returned by GET /v1/models.
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type modelsResponse struct {
	Data []Model `json:"data"`
}

// FetchModels calls GET <baseURL>/v1/models with an optional bearer token
func FetchModels(ctx context.Context, client *http.Client, baseURL, bearerToken string) ([]Model, error) {
	addr := strings.TrimRight(baseURL, "/") + "/v1/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr, nil)
	if err != nil {
		return nil, fmt.Errorf("preparing %q request: %w", addr, err)
	}
	if strings.TrimSpace(bearerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing %q request: %w", addr, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("%q returned %s: %w", addr, resp.Status, ErrUnauthorized)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, fmt.Errorf("%q returned %s", addr, resp.Status)
	}

	var payload modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("parsing %q response: %w", addr, err)
	}

	return payload.Data, nil
}

// ModelIDs reduces a model list to its IDs, preserving order.
func ModelIDs(models []Model) []string {
	ids := make([]string, 0, len(models))
	for _, m := range models {
		ids = append(ids, m.ID)
	}
	return ids
}
