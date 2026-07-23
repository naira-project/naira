package openaicompat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Model mirrors the OpenAI-compatible model object returned by GET /v1/models.
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type modelsResponse struct {
	Data []Model `json:"data"`
}

// FetchModels calls GET <baseURL>/v1/models with an optional Bearer token and returns the parsed model list.
func FetchModels(ctx context.Context, client *http.Client, baseURL, bearerToken string) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("building /v1/models request: %w", err)
	}

	if strings.TrimSpace(bearerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling /v1/models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("/v1/models returned %s", resp.Status)
	}

	var payload modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding /v1/models response: %w", err)
	}

	return payload.Data, nil
}
