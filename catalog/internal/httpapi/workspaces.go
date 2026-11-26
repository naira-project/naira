package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/naira-project/naira/catalog/internal/auth/keycloak"
	"github.com/naira-project/naira/catalog/internal/clusterregistry"
	"github.com/naira-project/naira/catalog/internal/clusterregistry/connectiontest"
	"github.com/naira-project/naira/catalog/internal/clusters"
)

var errInvalidManagementRequest = errors.New("invalid request")

func registerWorkspaceRoutes(router chi.Router, registry *clusters.Registry) {
	registrations := registry.Registrations
	router.Get("/workspaces", newListWorkspacesHandler(registrations))
	router.Route("/workspaces/{workspaceID}", func(r chi.Router) {
		r.Use(newWorkspaceAccessMiddleware(registrations))
		r.Get("/", newGetWorkspaceHandler())
		r.Get("/registrations", newListRegistrationsHandler(registrations))
		r.Post("/registrations", newCreateRegistrationHandler(registrations))
		r.Get("/registrations/{registrationID}", newGetRegistrationHandler(registrations))
		r.Post("/registrations/{registrationID}:test", newTestClusterConnectionHandler(registry.ConnectionTests))
		r.Put("/registrations/{registrationID}", newUpdateRegistrationHandler(registrations))
	})
}

type workspaceResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type kubernetesConnection struct {
	APIEndpoint string `json:"apiEndpoint"`
	Context     string `json:"context,omitempty"`
}

type registrationRequest struct {
	Name          string               `json:"name"`
	CredentialRef string               `json:"credentialRef"`
	Kubernetes    kubernetesConnection `json:"kubernetes"`
}

// registrationResource deliberately excludes credential references and values.
type registrationResource struct {
	ID               uuid.UUID                         `json:"id"`
	WorkspaceID      string                            `json:"workspaceId"`
	Name             string                            `json:"name"`
	Kubernetes       kubernetesConnection              `json:"kubernetes"`
	KubeSystemUID    string                            `json:"kubeSystemUid,omitempty"`
	ValidationStatus clusterregistry.ValidationStatus  `json:"validationStatus"`
	LastValidationAt *time.Time                        `json:"lastValidationAt,omitempty"`
	LastValidation   *clusterregistry.ValidationResult `json:"lastValidation,omitempty"`
	CreatedAt        time.Time                         `json:"createdAt"`
	UpdatedAt        time.Time                         `json:"updatedAt"`
}

func requestCaller(r *http.Request) clusterregistry.Caller {
	claims := keycloak.ClaimsFromContext(r.Context())
	return clusterregistry.Caller{
		Issuer:  claims.Issuer,
		Subject: claims.UserID,
	}
}

func testWorkspaceResponse() workspaceResource {
	return workspaceResource{
		ID:   clusterregistry.TestWorkspaceID,
		Name: clusterregistry.TestWorkspaceID,
	}
}

func registrationResponse(registration clusterregistry.Cluster) registrationResource {
	return registrationResource{
		ID:          registration.ID,
		WorkspaceID: registration.Workspace.ID,
		Name:        registration.Name,
		Kubernetes: kubernetesConnection{
			APIEndpoint: registration.Kubernetes.Connection.APIEndpoint,
			Context:     registration.Kubernetes.Connection.Context,
		},
		KubeSystemUID:    registration.Kubernetes.KubeSystemUID,
		ValidationStatus: registration.ValidationStatus,
		LastValidationAt: registration.LastValidationAt,
		LastValidation:   registration.LastValidation,
		CreatedAt:        registration.CreatedAt,
		UpdatedAt:        registration.UpdatedAt,
	}
}

func connectionParams(connection kubernetesConnection) clusterregistry.KubernetesConnection {
	return clusterregistry.KubernetesConnection{
		APIEndpoint: connection.APIEndpoint,
		Context:     connection.Context,
	}
}

func decodeManagementRequest(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode request body: %w", errInvalidManagementRequest)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("request must contain one JSON object: %w", errInvalidManagementRequest)
	}
	return nil
}

func managementHandler(next routeHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if err := next(w, r); err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, clusterregistry.ErrUnauthenticated):
				status = http.StatusUnauthorized
			case errors.Is(err, clusterregistry.ErrNotFound), errors.Is(err, clusterregistry.ErrWorkspaceNotFound):
				status = http.StatusNotFound
			case errors.Is(err, clusterregistry.ErrAlreadyExists):
				status = http.StatusConflict
			case errors.Is(err, clusterregistry.ErrValidationConflict):
				status = http.StatusConflict
			case errors.Is(err, connectiontest.ErrUnavailable):
				status = http.StatusServiceUnavailable
			case errors.Is(err, clusterregistry.ErrInvalidCluster), errors.Is(err, clusterregistry.ErrInvalidWorkspace), errors.Is(err, errInvalidManagementRequest):
				status = http.StatusBadRequest
			}
			writeJSON(w, status, map[string]string{"error": http.StatusText(status)})
		}
	}
}

// newListWorkspacesHandler returns the fixed workspace available to authenticated callers.
func newListWorkspacesHandler(clusterService *clusterregistry.Service) http.HandlerFunc {
	return managementHandler(func(w http.ResponseWriter, r *http.Request) error {
		if err := clusterService.CheckAccess(r.Context(), requestCaller(r), clusterregistry.Workspace{ID: clusterregistry.TestWorkspaceID}); err != nil {
			return fmt.Errorf("access test workspace: %w", err)
		}
		writeJSON(w, http.StatusOK, map[string]any{"workspaces": []workspaceResource{testWorkspaceResponse()}})
		return nil
	})
}

// newWorkspaceAccessMiddleware checks the workspace before parsing request data.
func newWorkspaceAccessMiddleware(clusterService *clusterregistry.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return managementHandler(func(w http.ResponseWriter, r *http.Request) error {
			if err := clusterService.CheckAccess(r.Context(), requestCaller(r), clusterregistry.Workspace{ID: chi.URLParam(r, "workspaceID")}); err != nil {
				return fmt.Errorf("authorize workspace: %w", err)
			}
			next.ServeHTTP(w, r)
			return nil
		})
	}
}

// newGetWorkspaceHandler returns the fixed workspace after workspace access is checked.
func newGetWorkspaceHandler() http.HandlerFunc {
	return managementHandler(func(w http.ResponseWriter, _ *http.Request) error {
		writeJSON(w, http.StatusOK, testWorkspaceResponse())
		return nil
	})
}

// newListRegistrationsHandler lists clusters registered in the requested workspace.
func newListRegistrationsHandler(clusterService *clusterregistry.Service) http.HandlerFunc {
	return managementHandler(func(w http.ResponseWriter, r *http.Request) error {
		list, err := clusterService.List(r.Context(), requestCaller(r), clusterregistry.Workspace{ID: chi.URLParam(r, "workspaceID")})
		if err != nil {
			return fmt.Errorf("list registrations: %w", err)
		}
		result := make([]registrationResource, 0, len(list))
		for _, registration := range list {
			result = append(result, registrationResponse(registration))
		}
		writeJSON(w, http.StatusOK, map[string]any{"registrations": result})
		return nil
	})
}

// newCreateRegistrationHandler creates a cluster registration without testing the connection.
func newCreateRegistrationHandler(clusterService *clusterregistry.Service) http.HandlerFunc {
	return managementHandler(func(w http.ResponseWriter, r *http.Request) error {
		var input registrationRequest
		if err := decodeManagementRequest(w, r, &input); err != nil {
			return fmt.Errorf("decode registration: %w", err)
		}
		registration, err := clusterService.Create(r.Context(), requestCaller(r), clusterregistry.CreateParams{
			Workspace:     clusterregistry.Workspace{ID: chi.URLParam(r, "workspaceID")},
			Name:          input.Name,
			CredentialRef: input.CredentialRef,
			Kubernetes:    connectionParams(input.Kubernetes),
		})
		if err != nil {
			return fmt.Errorf("create registration: %w", err)
		}
		w.Header().Set("Location", "/v1/workspaces/"+registration.Workspace.ID+"/registrations/"+registration.ID.String())
		writeJSON(w, http.StatusCreated, registrationResponse(registration))
		return nil
	})
}

// newGetRegistrationHandler returns one cluster registration without credential references.
func newGetRegistrationHandler(clusterService *clusterregistry.Service) http.HandlerFunc {
	return managementHandler(func(w http.ResponseWriter, r *http.Request) error {
		id, err := uuid.Parse(chi.URLParam(r, "registrationID"))
		if err != nil {
			return fmt.Errorf("parse registration ID: %w", errInvalidManagementRequest)
		}
		registration, err := clusterService.Get(r.Context(), requestCaller(r), clusterregistry.Workspace{ID: chi.URLParam(r, "workspaceID")}, id)
		if err != nil {
			return fmt.Errorf("get registration: %w", err)
		}
		writeJSON(w, http.StatusOK, registrationResponse(registration))
		return nil
	})
}

// newTestClusterConnectionHandler tests a saved connection without running discovery.
func newTestClusterConnectionHandler(connectionTestService *connectiontest.Service) http.HandlerFunc {
	return managementHandler(func(w http.ResponseWriter, r *http.Request) error {
		id, err := uuid.Parse(chi.URLParam(r, "registrationID"))
		if err != nil {
			return fmt.Errorf("parse registration ID: %w", errInvalidManagementRequest)
		}
		result, err := connectionTestService.TestConnection(r.Context(), requestCaller(r), clusterregistry.Workspace{ID: chi.URLParam(r, "workspaceID")}, id)
		if err != nil {
			return fmt.Errorf("test cluster connection: %w", err)
		}
		writeJSON(w, http.StatusOK, result)
		return nil
	})
}

// newUpdateRegistrationHandler replaces editable cluster registration fields.
func newUpdateRegistrationHandler(clusterService *clusterregistry.Service) http.HandlerFunc {
	return managementHandler(func(w http.ResponseWriter, r *http.Request) error {
		id, err := uuid.Parse(chi.URLParam(r, "registrationID"))
		if err != nil {
			return fmt.Errorf("parse registration ID: %w", errInvalidManagementRequest)
		}
		var input registrationRequest
		if err := decodeManagementRequest(w, r, &input); err != nil {
			return fmt.Errorf("decode update: %w", err)
		}
		if err := clusterService.Update(r.Context(), requestCaller(r), clusterregistry.Workspace{ID: chi.URLParam(r, "workspaceID")}, id, clusterregistry.UpdateParams{
			Name:          input.Name,
			CredentialRef: input.CredentialRef,
			Kubernetes:    connectionParams(input.Kubernetes),
		}); err != nil {
			return fmt.Errorf("update registration: %w", err)
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}
