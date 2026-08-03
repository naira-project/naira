package keycloak

import (
	"context"
	"encoding/json"
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
			if kc.Client == nil {
				writeJSONError(w, http.StatusInternalServerError, "auth is not configured")
				return
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeJSONError(w, http.StatusUnauthorized, "missing authorization header")
				return
			}

			before, after, found := strings.Cut(authHeader, " ")
			if !found || !strings.EqualFold(before, "Bearer") {
				writeJSONError(w, http.StatusUnauthorized, "authorization header must be Bearer {token}")
				return
			}
			tokenString := strings.TrimSpace(after)

			_, rawClaims, err := kc.Client.DecodeAccessToken(r.Context(), tokenString, kc.Realm)
			if err != nil || rawClaims == nil {
				writeJSONError(w, http.StatusUnauthorized, "invalid token")
				return
			}

			tc := parseTokenClaims(rawClaims)
			if tc.UserID == "" {
				writeJSONError(w, http.StatusUnauthorized, "invalid token")
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, tc)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
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
