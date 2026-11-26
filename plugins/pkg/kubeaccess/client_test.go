package kubeaccess

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClientDoesNotContactAPI(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("client initialization must not contact Kubernetes")
	}))
	defer server.Close()
	for _, contextName := range []string{"", "test"} {
		t.Run("context="+contextName, func(t *testing.T) {
			client, check := newClient(kubeconfig(t, server, nil), server.URL, contextName)
			require.NotNil(t, client)
			assert.Equal(t, Check{
				Name:    "configuration",
				Passed:  true,
				Code:    "OK",
				Message: "Selected kubeconfig and endpoint accepted.",
			}, check)
		})
	}
}
