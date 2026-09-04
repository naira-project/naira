package repositoryidentity

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	// Format SCP: git@github.com:owner/repo[.git]
	scpPattern = regexp.MustCompile(`^git@github\.com:([^/]+)/([^/]+)$`)
)

// ParseGitHubRepository returns the owner and repository name from a GitHub URL.
func ParseGitHubRepository(rawURL string) (owner, name string, ok bool) {
	input := strings.TrimSpace(rawURL)
	if input == "" {
		return "", "", false
	}

	if strings.HasPrefix(input, "git@") {
		m := scpPattern.FindStringSubmatch(input)
		if m == nil {
			return "", "", false
		}
		return validate(m[1], m[2])
	}

	if !strings.Contains(input, "://") {
		input = "https://" + input
	}

	u, err := url.Parse(input)
	if err != nil {
		return "", "", false
	}

	switch {
	case u.Scheme != "http" && u.Scheme != "https":
		return "", "", false
	case u.User != nil:
		return "", "", false
	case u.Port() != "":
		return "", "", false
	case u.RawQuery != "" || u.Fragment != "":
		return "", "", false
	case !strings.EqualFold(u.Hostname(), "github.com"):
		return "", "", false
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 {
		return "", "", false
	}

	return validate(parts[0], parts[1])
}

func validate(rawOwner, rawRepo string) (owner, name string, ok bool) {
	repo := strings.TrimSuffix(rawRepo, ".git")
	if repo == "." || repo == ".." {
		return "", "", false
	}
	return rawOwner, repo, true
}

// GitHubRepositoryNodePath returns the stable graph path for a GitHub repository.
func GitHubRepositoryNodePath(owner, name string) string {
	if owner == "" || name == "" {
		return ""
	}
	return "github.com/" + strings.ToLower(owner+"/"+name)
}

// GitHubRepositoryNodePathFromURL returns the stable graph path for a GitHub
// repository URL, or an empty string when the URL is unsupported.
func GitHubRepositoryNodePathFromURL(rawURL string) string {
	owner, name, ok := ParseGitHubRepository(rawURL)
	if !ok {
		return ""
	}
	return GitHubRepositoryNodePath(owner, name)
}
