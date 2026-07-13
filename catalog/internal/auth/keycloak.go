package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const claimsKey contextKey = "claims"

type TokenDecoder interface {
	DecodeAccessToken(ctx context.Context, accessToken, realm string) (*jwt.Token, *jwt.MapClaims, error)
}

// KeycloakConfig holds the client and realm needed for token verification.
type KeycloakConfig struct {
	Client TokenDecoder
	Realm  string
}

// TokenClaims holds the user identity extracted from a verified Keycloak JWT.
type TokenClaims struct {
	Sub        string
	Email      string
	Username   string
	RealmRoles []string
}

type keycloakClaims struct {
	jwt.RegisteredClaims
	Email             string `json:"email"`
	PreferredUsername string `json:"preferred_username"`
	RealmAccess       struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
}

func NewAuthMiddleware(kc KeycloakConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				http.Error(w, "authorization header must be Bearer {token}", http.StatusUnauthorized)
				return
			}
			tokenString := parts[1]

			_, rawClaims, err := kc.Client.DecodeAccessToken(r.Context(), tokenString, kc.Realm)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid token: %v", err), http.StatusUnauthorized)
				return
			}

			tc := parseTokenClaims(rawClaims)
			ctx := context.WithValue(r.Context(), claimsKey, tc)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func ClaimsFromContext(ctx context.Context) (TokenClaims, bool) {
	tc, ok := ctx.Value(claimsKey).(TokenClaims)
	return tc, ok
}

func parseTokenClaims(rawClaims *jwt.MapClaims) TokenClaims {
	claims := *rawClaims
	tc := TokenClaims{
		Sub:      stringClaim(claims, "sub"),
		Email:    stringClaim(claims, "email"),
		Username: stringClaim(claims, "preferred_username"),
	}

	if ra, ok := claims["realm_access"].(map[string]interface{}); ok {
		if roles, ok := ra["roles"].([]interface{}); ok {
			for _, r := range roles {
				if s, ok := r.(string); ok {
					tc.RealmRoles = append(tc.RealmRoles, s)
				}
			}
		}
	}

	return tc
}

func stringClaim(claims map[string]interface{}, key string) string {
	if v, ok := claims[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
