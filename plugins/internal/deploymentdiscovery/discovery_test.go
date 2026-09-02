package deploymentdiscovery

import (
	"testing"

	"github.com/naira-project/naira/plugins/internal/sourcerepository"
)

func TestGitHubRepositoryNodePathFromReference(t *testing.T) {
	tests := []struct {
		name string
		repo sourcerepository.Repository
		want string
	}{
		{
			name: "parsed GitHub owner and name include canonical host",
			repo: sourcerepository.Repository{Owner: "Acme", Name: "Service"},
			want: "github.com/acme/service",
		},
		{
			name: "GitHub URL includes canonical host",
			repo: sourcerepository.Repository{URL: "git@github.com:Acme/Service.git"},
			want: "github.com/acme/service",
		},
		{
			name: "unsupported GitLab URL is not canonicalized",
			repo: sourcerepository.Repository{URL: "https://gitlab.com/acme/service.git"},
			want: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := GitHubRepositoryNodePathFromReference(test.repo); got != test.want {
				t.Fatalf("GitHubRepositoryNodePathFromReference(%+v) = %q, want %q", test.repo, got, test.want)
			}
		})
	}
}
