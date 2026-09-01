package repositorydiscovery

import "testing"

func TestCanonicalPathFromRepo(t *testing.T) {
	tests := []struct {
		name string
		repo Repository
		want string
	}{
		{
			name: "parsed GitHub owner and name include canonical host",
			repo: Repository{Owner: "Acme", Name: "Service"},
			want: "github.com/acme/service",
		},
		{
			name: "GitHub URL includes canonical host",
			repo: Repository{URL: "git@github.com:Acme/Service.git"},
			want: "github.com/acme/service",
		},
		{
			name: "unsupported GitLab URL is not canonicalized",
			repo: Repository{URL: "https://gitlab.com/acme/service.git"},
			want: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := CanonicalPathFromRepo(test.repo); got != test.want {
				t.Fatalf("CanonicalPathFromRepo(%+v) = %q, want %q", test.repo, got, test.want)
			}
		})
	}
}
