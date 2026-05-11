// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and naira contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package openmetadata implements the translation adapter between OpenMetadata
// and the Naira Dataset CRD. This package is the ONLY place in the Naira
// codebase that is aware of OpenMetadata-specific types and APIs. All other
// Naira components interact exclusively with the generic Dataset resource.
package openmetadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Table represents a minimal view of an OpenMetadata Table entity as returned
// by the /api/v1/tables endpoint. Only fields required for Dataset mapping are
// included here; additional OpenMetadata fields are intentionally ignored to
// preserve the adapter's narrow scope.
type Table struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	FullyQualifiedName string `json:"fullyQualifiedName"`
	Description string    `json:"description"`
	Owner       *OMOwner  `json:"owner,omitempty"`
	Columns     []OMColumn `json:"columns,omitempty"`
	Tags        []OMTag   `json:"tags,omitempty"`
}

// OMOwner is the OpenMetadata owner sub-entity.
type OMOwner struct {
	Name string `json:"name"`
	Type string `json:"type"` // "user" or "team"
}

// OMColumn is a single column entry in an OpenMetadata table.
type OMColumn struct {
	Name        string  `json:"name"`
	DataType    string  `json:"dataType"`
	Description string  `json:"description"`
	Tags        []OMTag `json:"tags,omitempty"`
}

// OMTag is an OpenMetadata tag or classification label.
type OMTag struct {
	TagFQN      string `json:"tagFQN"`
	Source      string `json:"source"`
	LabelType   string `json:"labelType"`
}

// tablesResponse is the paginated response envelope from /api/v1/tables.
type tablesResponse struct {
	Data   []Table `json:"data"`
	Paging struct {
		After  string `json:"after,omitempty"`
		Total  int    `json:"total"`
	} `json:"paging"`
}

// Client is a lightweight HTTP client for the OpenMetadata REST API.
// It handles authentication and pagination transparently.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new OpenMetadata API client.
// baseURL must point to the root of the OpenMetadata instance (e.g., "https://openmetadata.example.com").
// token is the JWT bearer token obtained from a Kubernetes Secret.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ListTables retrieves all Table entities from OpenMetadata, following
// pagination automatically. It returns the full set of tables or an error.
func (c *Client) ListTables(ctx context.Context) ([]Table, error) {
	var all []Table
	after := ""

	for {
		batch, nextAfter, err := c.listTablesBatch(ctx, after)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if nextAfter == "" {
			break
		}
		after = nextAfter
	}

	return all, nil
}

// listTablesBatch fetches a single page of tables. It returns the tables, the
// cursor for the next page (empty string when there are no more pages), and any
// error.
func (c *Client) listTablesBatch(ctx context.Context, after string) ([]Table, string, error) {
	endpoint, err := url.JoinPath(c.baseURL, "/api/v1/tables")
	if err != nil {
		return nil, "", fmt.Errorf("building tables URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", fmt.Errorf("creating request: %w", err)
	}

	q := req.URL.Query()
	q.Set("fields", "owner,columns,tags")
	q.Set("limit", "100")
	if after != "" {
		q.Set("after", after)
	}
	req.URL.RawQuery = q.Encode()

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("unexpected status %d from OpenMetadata: %s", resp.StatusCode, body)
	}

	var result tablesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, "", fmt.Errorf("decoding response: %w", err)
	}

	return result.Data, result.Paging.After, nil
}

// TableURL returns the deep-link URL for a specific table within the
// OpenMetadata web UI.
func (c *Client) TableURL(fqn string) string {
	return fmt.Sprintf("%s/table/%s", c.baseURL, url.PathEscape(fqn))
}
