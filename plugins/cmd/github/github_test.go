package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	codeowners := "* @acme/team\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(codeowners))

	tests := []struct {
		name         string
		foundAt      string
		encoding     string
		content      string
		wantFound    bool
		wantContent  string
		wantRequests []string
	}{
		{
			name:         "uses the first available location",
			foundAt:      "/repos/acme/service/contents/CODEOWNERS",
			encoding:     "base64",
			content:      encoded,
			wantFound:    true,
			wantContent:  codeowners,
			wantRequests: []string{".github/CODEOWNERS", "CODEOWNERS"},
		},
		{
			name:         "returns not found when no location exists",
			wantRequests: []string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"},
		},
		{
			name:         "skips unsupported encoding and continues",
			foundAt:      "/repos/acme/service/contents/docs/CODEOWNERS",
			encoding:     "text",
			content:      codeowners,
			wantRequests: []string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.URL.Path)
				if r.URL.Path != tt.foundAt {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":%q}`, tt.content, tt.encoding)
			}))
			t.Cleanup(srv.Close)

			got, err := newGithubClient(srv.Client(), srv.URL, "").GetCodeowners(context.Background(), "acme", "service")
			if tt.wantFound {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.True(t, errors.Is(err, errGithubResourceNotFound))
			}
			assert.Equal(t, tt.wantContent, got)
			assert.Equal(t, expectedCodeownersPaths(tt.wantRequests), requests)
		})
	}
}

func expectedCodeownersPaths(names []string) []string {
	paths := make([]string, 0, len(names))
	for _, name := range names {
		paths = append(paths, "/repos/acme/service/contents/"+name)
	}
	return paths
}
