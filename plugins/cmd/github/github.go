package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type githubClient struct {
	httpClient *http.Client
	baseURL    string // e.g. https://api.github.com
	token      string
}

type ghRepo struct {
	HTMLURL  string `json:"html_url"`
	Language string `json:"language"`
	Homepage string `json:"homepage"`
}

type ghContent struct {
	Content  string `json:"content"` // base64, when Encoding == "base64"
	Encoding string `json:"encoding"`
}

func newGithubClient(httpClient *http.Client, baseURL, token string) *githubClient {
	return &githubClient{
		httpClient: httpClient,
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		token:      token,
	}
}

func (c *githubClient) do(ctx context.Context, path string, out any) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return false, fmt.Errorf("building github request for %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("calling github api %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("github api %s returned status %d", path, resp.StatusCode)
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return false, fmt.Errorf("decoding github api response for %s: %w", path, err)
		}
	}
	return true, nil
}

func (c *githubClient) GetRepo(ctx context.Context, owner, repo string) (ghRepo, bool, error) {
	var r ghRepo
	found, err := c.do(ctx, fmt.Sprintf("/repos/%s/%s", url.PathEscape(owner), url.PathEscape(repo)), &r)
	if err != nil {
		return ghRepo{}, false, fmt.Errorf("getting repo %s/%s: %w", owner, repo, err)
	}
	return r, found, nil
}

// GetCodeowners tries the well-known CODEOWNERS locations, in the order
// GitHub itself checks them, and returns the content of the first one found.
func (c *githubClient) GetCodeowners(ctx context.Context, owner, repo string) (string, bool, error) {
	candidates := []string{"CODEOWNERS", ".github/CODEOWNERS", "docs/CODEOWNERS"}

	for _, path := range candidates {
		var content ghContent
		found, err := c.do(ctx, fmt.Sprintf("/repos/%s/%s/contents/%s", url.PathEscape(owner), url.PathEscape(repo), path), &content)
		if err != nil {
			return "", false, fmt.Errorf("getting %s for %s/%s: %w", path, owner, repo, err)
		}
		if !found {
			continue
		}
		if content.Encoding != "base64" {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content.Content, "\n", ""))
		if err != nil {
			return "", false, fmt.Errorf("decoding %s for %s/%s: %w", path, owner, repo, err)
		}
		return string(decoded), true, nil
	}

	return "", false, nil
}
