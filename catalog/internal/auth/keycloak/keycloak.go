package keycloak

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

// Config holds the client and realm needed for token verification.
type Config struct {
	Client TokenDecoder
	Realm  string
}

// TokenClaims holds the user identity extracted from a verified Keycloak JWT.
type TokenClaims struct {
	UserID     string
	Email      string
	Username   string
	RealmRoles []string
}

func NewAuthMiddleware(kc Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}

			before, after, found := strings.Cut(authHeader, " ")
			if !found || !strings.EqualFold(before, "Bearer") {
				http.Error(w, "authorization header must be Bearer {token}", http.StatusUnauthorized)
				return
			}
			tokenString := strings.TrimSpace(after)

			_, rawClaims, err := kc.Client.DecodeAccessToken(r.Context(), tokenString, kc.Realm)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid token"), http.StatusUnauthorized)
				return
			}

			tc := parseTokenClaims(rawClaims)
			ctx := context.WithValue(r.Context(), claimsKey, tc)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func claimsFromContext(ctx context.Context) (TokenClaims, bool) {
	tc, ok := ctx.Value(claimsKey).(TokenClaims)
	return tc, ok
}

func parseTokenClaims(rawClaims *jwt.MapClaims) TokenClaims {
	claims := *rawClaims
	tc := TokenClaims{
		UserID:   stringClaim(claims, "sub"),
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
