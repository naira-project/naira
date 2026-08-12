package keycloak

import (
	"context"
	"encoding/json"
	"errors"
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
	PreferredUsername   string
}

func NewAuthMiddleware(cfg Config) (func(http.Handler) http.Handler, error) {
	if cfg.Client == nil {
		return nil, errors.New("keycloak: Config.Client must not be nil")
	}

	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

			_, rawClaims, err := cfg.Client.DecodeAccessToken(r.Context(), tokenString, cfg.Realm)
			if err != nil || rawClaims == nil {
				writeJSONError(w, http.StatusUnauthorized, "invalid token")
				return
			}

			claims := parseTokenClaims(rawClaims)
			if claims.UserID == "" {
				writeJSONError(w, http.StatusUnauthorized, "invalid token")
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	return middleware, nil
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func claimsFromContext(ctx context.Context) (TokenClaims, bool) {
	claims, ok := ctx.Value(claimsKey).(TokenClaims)
	return claims, ok
}

func parseTokenClaims(rawClaims *jwt.MapClaims) TokenClaims {
	claims := *rawClaims
	tc := TokenClaims{
		UserID:   stringClaim(claims, "sub"),
		Email:    stringClaim(claims, "email"),
		PreferredUsername: stringClaim(claims, "preferred_username"),
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
