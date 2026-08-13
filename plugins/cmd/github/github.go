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

// githubClient is a deliberately small REST client — just enough to cover
// what this plugin needs. Pulling in a full SDK (e.g. go-github) is more
// than this warrants right now; if the plugin grows (GitLab, more
// endpoints), revisit.
type githubClient struct {
	httpClient *http.Client
	baseURL    string // e.g. https://api.github.com
	token      string
}

type ghRepo struct {
	Name            string   `json:"name"`
	FullName        string   `json:"full_name"` // "owner/repo"
	Description     string   `json:"description"`
	HTMLURL         string   `json:"html_url"`
	DefaultBranch   string   `json:"default_branch"`
	Language        string   `json:"language"`
	Archived        bool     `json:"archived"`
	Fork            bool     `json:"fork"`
	Topics          []string `json:"topics"`
	StargazersCount int      `json:"stargazers_count"`
	Homepage        string   `json:"homepage"`
}

type ghCommit struct {
	Commit struct {
		Committer struct {
			Date string `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
}

type ghRelease struct {
	TagName     string `json:"tag_name"`
	PublishedAt string `json:"published_at"`
	HTMLURL     string `json:"html_url"`
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

// ListOrgRepos lists repositories for an org. Single page only (per_page=100);
// orgs with >100 repos need pagination added here later.
func (c *githubClient) ListOrgRepos(ctx context.Context, org string) ([]ghRepo, error) {
	var repos []ghRepo
	found, err := c.do(ctx, fmt.Sprintf("/orgs/%s/repos?per_page=100&type=sources", url.PathEscape(org)), &repos)
	if err != nil {
		return nil, fmt.Errorf("listing repos for org %s: %w", org, err)
	}
	if !found {
		return nil, nil
	}
	return repos, nil
}

func (c *githubClient) GetRepo(ctx context.Context, owner, repo string) (ghRepo, bool, error) {
	var r ghRepo
	found, err := c.do(ctx, fmt.Sprintf("/repos/%s/%s", url.PathEscape(owner), url.PathEscape(repo)), &r)
	if err != nil {
		return ghRepo{}, false, fmt.Errorf("getting repo %s/%s: %w", owner, repo, err)
	}
	return r, found, nil
}

// GetLatestCommitDate returns the commit date of the most recent commit on
// the repo's default branch, used as an activity/freshness signal.
func (c *githubClient) GetLatestCommitDate(ctx context.Context, owner, repo string) (string, bool, error) {
	var commits []ghCommit
	found, err := c.do(ctx, fmt.Sprintf("/repos/%s/%s/commits?per_page=1", url.PathEscape(owner), url.PathEscape(repo)), &commits)
	if err != nil {
		return "", false, fmt.Errorf("getting latest commit for %s/%s: %w", owner, repo, err)
	}
	if !found || len(commits) == 0 {
		return "", false, nil
	}
	return commits[0].Commit.Committer.Date, true, nil
}

func (c *githubClient) GetLatestRelease(ctx context.Context, owner, repo string) (ghRelease, bool, error) {
	var r ghRelease
	found, err := c.do(ctx, fmt.Sprintf("/repos/%s/%s/releases/latest", url.PathEscape(owner), url.PathEscape(repo)), &r)
	if err != nil {
		return ghRelease{}, false, fmt.Errorf("getting latest release for %s/%s: %w", owner, repo, err)
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
