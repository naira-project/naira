package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Nerzal/gocloak/v13"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/auth"
)

type routeHandler func(http.ResponseWriter, *http.Request) error
type listOptionsHandler func(http.ResponseWriter, *http.Request, listOptions) error

type contextKey string

const claimsKey contextKey = "claims"

const (
	fgaModelType   = "naira_io_model"
	fgaGetRelation = "get"
)

// KeycloakConfig holds the gocloak client and realm needed for token verification.
type KeycloakConfig struct {
	Client *gocloak.GoCloak
	Realm  string
	ClientID string
}

// TokenClaims holds the user identity extracted from a verified Keycloak JWT.
type TokenClaims struct {
	Sub      string
	Email    string
	Username string
	RealmRoles  []string
	ClientRoles []string
}

type keycloakClaims struct {
	jwt.RegisteredClaims
	Email             string         `json:"email"`
	PreferredUsername string         `json:"preferred_username"`
	RealmAccess       struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
	ResourceAccess map[string]struct {
		Roles []string `json:"roles"`
	} `json:"resource_access"`
}


func newAuthMiddleware(kc KeycloakConfig) func(http.Handler) http.Handler {
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

			tc := parseTokenClaims(rawClaims, kc.ClientID)
			ctx := context.WithValue(r.Context(), claimsKey, tc)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func parseTokenClaims(rawClaims *jwt.MapClaims, clientID string) TokenClaims {
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

	if ra, ok := claims["resource_access"].(map[string]interface{}); ok {
		if client, ok := ra[clientID].(map[string]interface{}); ok {
			if roles, ok := client["roles"].([]interface{}); ok {
				for _, r := range roles {
					if s, ok := r.(string); ok {
						tc.ClientRoles = append(tc.ClientRoles, s)
					}
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

func claimsFromContext(ctx context.Context) (TokenClaims, bool) {
	tc, ok := ctx.Value(claimsKey).(TokenClaims)
	return tc, ok
}

// authorizeNodeRead checks the existing OpenFGA tuples to determine whether the
// authenticated caller has the "get" relation on the requested node.
func authorizeNodeRead(ctx context.Context, node catalog.NodeID) error {
	claims, ok := claimsFromContext(ctx)
	if !ok || claims.Sub == "" {
		return fmt.Errorf("no authenticated user in request context")
	}

	fgaClient, err := auth.GetClient()
	if err != nil {
		return fmt.Errorf("openfga client not configured: %w", err)
	}

	modelID, err := auth.GetModelID()
	if err != nil {
		return fmt.Errorf("getting openfga model id: %w", err)
	}

	object := fmt.Sprintf("%s:%s/%s", fgaModelType, node.Kind, node.Path)
	allowed, err := auth.CheckTuples(fgaClient, "user:"+claims.Sub, fgaGetRelation, object, modelID)
	if err != nil {
		return fmt.Errorf("checking openfga tuples: %w", err)
	}

	if !allowed.Allowed {
		return fmt.Errorf("user %q is not allowed to %s %s", claims.Sub, fgaGetRelation, object)
	}

	return nil
}

func NewRouter(service *catalog.Service, logger *log.Logger, kc KeycloakConfig) http.Handler {
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID)
	router.Use(chimiddleware.Recoverer)
	if logger != nil {
		router.Use(requestLogger(logger))
	}

	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	router.Route("/v1", func(r chi.Router) {
			r.Use(newAuthMiddleware(kc))

			r.Post("/plugins:run", handle(func(w http.ResponseWriter, r *http.Request) error {
				response := service.RunAllPlugins(r.Context())
				writeJSON(w, http.StatusAccepted, runPluginsResponseFromResult(response))
				return nil
			}))

			// GET /v1/nodes lists catalog nodes.
			// Supported query params:
			// - pageSize
			// - pageToken
			// - filter: only field="value" equality filters
			// Supported node filter fields: name, kind, path.
			r.Get("/nodes", handleWithListOptions(nodeListOptionsSpec, func(w http.ResponseWriter, r *http.Request, options listOptions) error {

				nodes := make([]Node, 0)
				for _, catalogNode := range service.ListNodes(r.Context()) {
					node := nodeFromCatalogNode(catalogNode)
					matches, err := matchNodeFilter(node, options.filter)
					if err != nil {
						return fmt.Errorf("matching node filter: %w", err)
					}
					if !matches {
						continue
					}
					if err := authorizeNodeRead(r.Context(), catalogNode.ID); err != nil {
						continue
					}
					nodes = append(nodes, node)
				}

				sortNodes(nodes)

				page, nextPageToken, totalSize, err := paginate(nodes, options.pageSize, options.offset, "nodes", logger)
				if err != nil {
					return fmt.Errorf("paginating nodes: %w", err)
				}

				writeJSON(w, http.StatusOK, ListNodesResponse{Nodes: page, NextPageToken: nextPageToken, TotalSize: int32FromCount(totalSize, logger)})
				return nil
			}))

			r.Get("/nodes/{kind}/*", handle(func(w http.ResponseWriter, r *http.Request) error {
				nodeID := catalog.NodeID{Kind: chi.URLParam(r, "kind"), Path: chi.URLParam(r, "*")}

				if err := authorizeNodeRead(r.Context(), nodeID); err != nil {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
					return fmt.Errorf("Status forbidden: %w", err)
				}

				node, err := service.GetNode(r.Context(), nodeID)
				if err != nil {
					return fmt.Errorf("getting node: %w", err)
				}

				writeJSON(w, http.StatusOK, nodeFromCatalogNode(node))
				return nil
			}))

			// GET /v1/relations lists catalog relations.
			// Supported query params:
			// - pageSize
			// - pageToken
			// - filter: only field="value" equality filters
			// Supported relation filter fields: name, kind, fromNode, toNode.
			r.Get("/relations", handleWithListOptions(relationListOptionsSpec, func(w http.ResponseWriter, r *http.Request, options listOptions) error {
				relations := make([]Relation, 0)
				for _, relation := range service.ListRelations(r.Context()) {
					resource := relationFromCatalogRelation(relation)
					matches, err := matchRelationFilter(resource, options.filter)
					if err != nil {
						return fmt.Errorf("matching relation filter: %w", err)
					}
					if matches {
						relations = append(relations, resource)
					}
				}

				sortRelations(relations)

				page, nextPageToken, totalSize, err := paginate(relations, options.pageSize, options.offset, "relations", logger)
				if err != nil {
					return fmt.Errorf("paginating relations: %w", err)
				}

				writeJSON(w, http.StatusOK, ListRelationsResponse{Relations: page, NextPageToken: nextPageToken, TotalSize: int32FromCount(totalSize, logger)})
				return nil
			}))
		})

	return router
}

func handle(next routeHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := next(w, r); err != nil {
			writeError(w, err)
		}
	}
}

func handleWithListOptions(spec listOptionsSpec, next listOptionsHandler) http.HandlerFunc {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		options, err := listOptionsFromRequest(r, spec)
		if err != nil {
			return fmt.Errorf("getting list options from request: %w", err)
		}

		return next(w, r, options)
	})
}

func requestLogger(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			logger.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	buf := &bytes.Buffer{}
	encoder := json.NewEncoder(buf)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := err.Error()
	switch {
	case errors.Is(err, catalog.ErrNodeNotFound):
		status = http.StatusNotFound
	case isClientRequestError(err):
		status = http.StatusBadRequest
	case errors.Is(err, errPageTokenEncoding):
		status = http.StatusInternalServerError
		message = http.StatusText(status)
	}

	writeJSON(w, status, map[string]string{"error": message})
}

func isClientRequestError(err error) bool {
	return errors.Is(err, errInvalidPageToken) ||
		errors.Is(err, errInvalidFilter) ||
		errors.Is(err, errInvalidOrderBy) ||
		errors.Is(err, catalog.ErrInvalidIngestion) ||
		errors.Is(err, errInvalidPageSize)
}
