package repositoryidentity

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	// scp-like syntax: git@github.com:owner/repo[.git]
	// Taken from: https://git-scm.com/docs/git-clone#_git_urls

	scpPattern = regexp.MustCompile(`^git@github\.com:([^/]+)/([^/]+)$`)

	// Handle rules: max 39 chars, alphanumeric + single hyphens (not at start or end)
	// Taken from: https://github.com/signup
	handlePattern = regexp.MustCompile(`^[a-zA-Z0-9]+(-[a-zA-Z0-9]+)*$`)
	handleMaxLen  = 39

	// Repo name pattern: max 100 chars, ASCII letters, digits, '.', '-', '_'
	// Taken from: https://docs.github.com/en/repositories/creating-and-managing-repositories/creating-a-new-repository#creating-a-new-repository-from-the-web-ui
	// Repo name "." and ".." is not allowed according to "https://github.com/new"
	repoPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	repoMaxLen  = 100
)

// ParseGitHubRepository returns the owner and repository name from a GitHub URL.
// It accepts HTTPS URLs, GitHub's scp-like SSH syntax, and URLs using the SSH
// scheme with the conventional git user.
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
	case u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "ssh":
		return "", "", false
	case u.Scheme == "ssh" && (u.User == nil || u.User.Username() != "git"):
		return "", "", false
	case u.Scheme != "ssh" && u.User != nil:
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
	if !isValidHandle(rawOwner) || !isValidRepoName(repo) {
		return "", "", false
	}
	return rawOwner, repo, true
}

func isValidHandle(h string) bool {
	return len(h) <= handleMaxLen && handlePattern.MatchString(h)
}

func isValidRepoName(r string) bool {
	return r != "." && r != ".." && len(r) <= repoMaxLen && repoPattern.MatchString(r)
}

// GitHubRepositoryNodePath returns a stable node path for a GitHub repository.
// owner and name must be non-empty and must not contain "/" — e.g. as
// returned by ParseGitHubRepository.
func GitHubRepositoryNodePath(owner, name string) string {
	return "github.com/" + strings.ToLower(owner+"/"+name)
}

// GitHubRepositoryNodePathFromURL returns a stable node path for a GitHub
// repository URL, or an empty string when the URL is unsupported.
func GitHubRepositoryNodePathFromURL(rawURL string) string {
	owner, name, ok := ParseGitHubRepository(rawURL)
	if !ok {
		return ""
	}
	return GitHubRepositoryNodePath(owner, name)
}
