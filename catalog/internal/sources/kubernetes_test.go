package sources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKubernetesConnectionValidate(t *testing.T) {
	tests := []struct {
		name       string
		connection KubernetesConnection
		wantErr    string
	}{
		{
			name:       "current kubeconfig context",
			connection: KubernetesConnection{APIEndpoint: "https://kubernetes.example.com:6443"},
		},
		{
			name: "explicit context and proxy path",
			connection: KubernetesConnection{
				APIEndpoint: "https://proxy.example.com/clusters/production",
				Context:     "production",
			},
		},
		{
			name:       "local HTTP proxy",
			connection: KubernetesConnection{APIEndpoint: "http://127.0.0.1:8001"},
		},
		{
			name:    "empty endpoint",
			wantErr: "API endpoint",
		},
		{
			name:       "surrounding whitespace",
			connection: KubernetesConnection{APIEndpoint: " https://kubernetes.example.com "},
			wantErr:    "API endpoint",
		},
		{
			name:       "control separators",
			connection: KubernetesConnection{APIEndpoint: "https://kubernetes.example.com/\nsensitive-value"},
			wantErr:    "API endpoint",
		},
		{
			name:       "relative endpoint",
			connection: KubernetesConnection{APIEndpoint: "kubernetes.example.com"},
			wantErr:    "API endpoint",
		},
		{
			name:       "unsupported scheme",
			connection: KubernetesConnection{APIEndpoint: "file:///etc/kubeconfig"},
			wantErr:    "API endpoint",
		},
		{
			name:       "missing host",
			connection: KubernetesConnection{APIEndpoint: "https:///api"},
			wantErr:    "API endpoint",
		},
		{
			name:       "malformed URL",
			connection: KubernetesConnection{APIEndpoint: "https://%"},
			wantErr:    "API endpoint",
		},
		{
			name:       "inline credentials",
			connection: KubernetesConnection{APIEndpoint: "https://user:sensitive-value@kubernetes.example.com"},
			wantErr:    "API endpoint",
		},
		{
			name:       "query",
			connection: KubernetesConnection{APIEndpoint: "https://kubernetes.example.com?token=sensitive-value"},
			wantErr:    "API endpoint",
		},
		{
			name:       "empty query",
			connection: KubernetesConnection{APIEndpoint: "https://kubernetes.example.com?"},
			wantErr:    "API endpoint",
		},
		{
			name:       "fragment",
			connection: KubernetesConnection{APIEndpoint: "https://kubernetes.example.com#sensitive-value"},
			wantErr:    "API endpoint",
		},
		{
			name:       "empty fragment",
			connection: KubernetesConnection{APIEndpoint: "https://kubernetes.example.com#"},
			wantErr:    "API endpoint",
		},
		{
			name: "context whitespace",
			connection: KubernetesConnection{
				APIEndpoint: "https://kubernetes.example.com",
				Context:     " production ",
			},
			wantErr: "kubeconfig context",
		},
		{
			name: "context control separators",
			connection: KubernetesConnection{
				APIEndpoint: "https://kubernetes.example.com",
				Context:     "production\nsensitive-value",
			},
			wantErr: "kubeconfig context",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.connection.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrInvalidRegistration)
			assert.ErrorContains(t, err, tt.wantErr)
			assert.NotContains(t, err.Error(), "sensitive-value")
		})
	}
}

func TestKubernetesDetailsValidate(t *testing.T) {
	tests := []struct {
		name    string
		status  ValidationStatus
		uid     string
		wantErr string
	}{
		{
			name:   "pending without fingerprint",
			status: ValidationPending,
		},
		{
			name:   "pending keeps verified fingerprint",
			status: ValidationPending,
			uid:    "installation-uid",
		},
		{
			name:   "failed before fingerprint was available",
			status: ValidationFailed,
		},
		{
			name:   "ready with fingerprint",
			status: ValidationReady,
			uid:    "installation-uid",
		},
		{
			name:    "ready requires fingerprint",
			status:  ValidationReady,
			wantErr: "requires a kube-system UID",
		},
		{
			name:    "fingerprint whitespace",
			status:  ValidationReady,
			uid:     " ",
			wantErr: "kube-system UID must not contain",
		},
		{
			name:    "fingerprint control separators",
			status:  ValidationPending,
			uid:     "installation\nuid",
			wantErr: "kube-system UID must not contain",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := KubernetesDetails{
				Connection:    KubernetesConnection{APIEndpoint: "https://kubernetes.example.com"},
				KubeSystemUID: tt.uid,
			}
			err := details.validate(tt.status)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrInvalidRegistration)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}
