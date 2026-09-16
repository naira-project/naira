package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGithubClient_GetRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/acme/service", r.URL.Path)
		assert.Equal(t, "Bearer secret-token", r.Header.Get("Authorization"))
		w.Write([]byte(`{"html_url":"https://github.com/acme/service","language":"Go"}`))
	}))
	defer srv.Close()

	c := newGithubClient(srv.Client(), srv.URL, "secret-token")
	repo, err := c.GetRepo(context.Background(), "acme", "service")
	require.NoError(t, err)
	assert.Equal(t, "Go", repo.Language)
}

func TestGithubClient_GetCodeowners(t *testing.T) {
	const codeownersBody = "* @acme/team\n"
	base64Body := base64.StdEncoding.EncodeToString([]byte(codeownersBody))

	tests := []struct {
		name         string
		responses    map[string]ghContent // path -> server response
		wantContent  string
		wantErr      error
		wantRequests []string // queried paths in chronological order
	}{
		{
			name: "found at first location",
			responses: map[string]ghContent{
				".github/CODEOWNERS": {Content: base64Body, Encoding: "base64"},
			},
			wantContent:  codeownersBody,
			wantRequests: []string{".github/CODEOWNERS"},
		},
		{
			name:         "not found anywhere",
			wantErr:      errGithubResourceNotFound,
			wantRequests: []string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"},
		},
		{
			name: "skips unsupported encoding and keeps looking",
			responses: map[string]ghContent{
				"docs/CODEOWNERS": {Content: codeownersBody, Encoding: "text"},
			},
			wantErr:      errGithubResourceNotFound,
			wantRequests: []string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests []string
			srv := newFakeGithubContentsServer(t, "acme", "service", tt.responses, &requests)

			got, err := newGithubClient(srv.Client(), srv.URL, "").GetCodeowners(context.Background(), "acme", "service")

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantContent, got)
			assert.Equal(t, tt.wantRequests, requests)
		})
	}
}

// newFakeGithubContentsServer simulates GET /repos/{owner}/{repo}/contents/{path}:
// records each requested path (relative to CODEOWNERS) into *requests and responds
// with content from `responses` for the given path, or 404 if not found
func newFakeGithubContentsServer(t *testing.T, owner, repo string, responses map[string]ghContent, requests *[]string) *httptest.Server {
	t.Helper()
	prefix := fmt.Sprintf("/repos/%s/%s/contents/", owner, repo)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		relPath := strings.TrimPrefix(r.URL.Path, prefix)
		*requests = append(*requests, relPath)

		content, ok := responses[relPath]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(content)
	}))
	t.Cleanup(srv.Close)
	return srv
}
