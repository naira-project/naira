// Package oidctest provides a lightweight stand-in for the slice of
// Keycloak that Bearer-auth middleware actually depends on: a JWKS endpoint
// and signed access tokens. It lets tests exercise real auth code without
// running a real Keycloak.
package oidctest

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oauth2-proxy/mockoidc"
	"github.com/stretchr/testify/require"
)

// Server serves a Keycloak-shaped JWKS endpoint
// (/realms/{realm}/protocol/openid-connect/certs), backed by a mockoidc
// keypair, without running mockoidc's own OIDC server or a real Keycloak.
type Server struct {
	// BaseURL is the server's root, suitable for e.g. KEYCLOAK_BASE_URL.
	BaseURL string
	// Issuer is the "iss" claim value tokens from this server carry,
	// suitable for e.g. KEYCLOAK_ISSUER.
	Issuer string

	keypair *mockoidc.Keypair
}

// New starts a Server for the given realm and registers its shutdown as test
// cleanup.
func New(t *testing.T, realm string) *Server {
	t.Helper()

	keypair, err := mockoidc.NewKeypair(nil)
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("/realms/"+realm+"/protocol/openid-connect/certs", func(w http.ResponseWriter, _ *http.Request) {
		jwks, err := keypair.JWKS()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwks)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &Server{
		BaseURL: srv.URL,
		Issuer:  srv.URL + "/realms/" + realm,
		keypair: keypair,
	}
}

// SignAccessToken returns a signed access token for the given subject, valid
// for one hour, with an "iss" claim matching s.Issuer.
func (s *Server) SignAccessToken(t *testing.T, subject string) string {
	t.Helper()

	now := time.Now()
	token, err := s.keypair.SignJWT(jwt.MapClaims{
		"iss":                s.Issuer,
		"sub":                subject,
		"email":              subject + "@example.com",
		"preferred_username": subject,
		"iat":                now.Unix(),
		"exp":                now.Add(time.Hour).Unix(),
	})
	require.NoError(t, err, "signing mock access token")
	return token
}
