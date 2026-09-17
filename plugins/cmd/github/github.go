package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

var errGithubResourceNotFound = errors.New("github resource not found")

type githubClient struct {
	httpClient *http.Client
	baseURL    string // e.g. https://api.github.com
	token      string
}

// ghRepo is information retrived from the GitHub API about a repository
// from GET /repos/{owner}/{repo}
// See: https://docs.github.com/en/rest/repos/repos#get-a-repository
type ghRepo struct {
	// HTMLURL - github repo url
	HTMLURL  string `json:"html_url"`
	Language string `json:"language"`
	// Homepage - project website. e.g. "https://naira-project.github.io/"
	Homepage string `json:"homepage"`
}

// ghContent represents a file response from GET /repos/{owner}/{repo}/contents/{path}.
// See: https://docs.github.com/en/rest/repos/contents#get-repository-content
type ghContent struct {
	// Content contains the base64-encoded file payload (punctuated with newlines '\n').
	Content string `json:"content"`

	// Encoding describes the content encoding scheme.
	// "base64" for files <= 1MB
	Encoding string `json:"encoding"`
}

func newGithubClient(httpClient *http.Client, baseURL, token string) *githubClient {
	return &githubClient{
		httpClient: httpClient,
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		token:      token,
	}
}

func (c *githubClient) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("building github request for %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling github api %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return errGithubResourceNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github api %s returned status %d", path, resp.StatusCode)
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decoding github api response for %s: %w", path, err)
		}
	}
	return nil
}

func (c *githubClient) GetRepo(ctx context.Context, owner, repo string) (ghRepo, error) {
	var githubRepo ghRepo
	err := c.get(ctx, fmt.Sprintf("/repos/%s/%s", url.PathEscape(owner), url.PathEscape(repo)), &githubRepo)
	if err != nil {
		return ghRepo{}, fmt.Errorf("getting repo %s/%s: %w", owner, repo, err)
	}
	return githubRepo, nil
}

// GetCodeowners tries the well-known CODEOWNERS locations, in the order
// GitHub itself checks them, and returns the content of the first one found.
// Order of locations: https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners#codeowners-file-location
// This function doesn't support files bigger than 1MB, as it would require handling a different encoding than base64.
func (c *githubClient) GetCodeowners(ctx context.Context, owner, repo string) (string, error) {
	candidates := []string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"}

	for _, path := range candidates {
		var content ghContent
		err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/contents/%s", url.PathEscape(owner), url.PathEscape(repo), path), &content)
		if errors.Is(err, errGithubResourceNotFound) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("getting %s for %s/%s: %w", path, owner, repo, err)
		}
		if content.Encoding != "base64" {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content.Content, "\n", ""))
		if err != nil {
			return "", fmt.Errorf("decoding %s for %s/%s: %w", path, owner, repo, err)
		}
		return string(decoded), nil
	}

	return "", errGithubResourceNotFound
}
