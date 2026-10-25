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
	"github.com/naira-project/naira/catalog/internal/sources"
	"github.com/naira-project/naira/catalog/internal/workspaces"
)

var errInvalidManagementRequest = errors.New("invalid request")

type workspaceResource struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type kubernetesConnection struct {
	APIEndpoint string `json:"apiEndpoint"`
	Context     string `json:"context,omitempty"`
}

type registrationRequest struct {
	Name          string                `json:"name"`
	Kind          sources.Kind          `json:"kind"`
	CredentialRef string                `json:"credentialRef"`
	Kubernetes    *kubernetesConnection `json:"kubernetes"`
}

type registrationUpdateRequest struct {
	Name          string                `json:"name"`
	CredentialRef string                `json:"credentialRef"`
	Kubernetes    *kubernetesConnection `json:"kubernetes"`
}

// registrationResource deliberately excludes credential references and values.
type registrationResource struct {
	ID               uuid.UUID                `json:"id"`
	WorkspaceID      string                   `json:"workspaceId"`
	Name             string                   `json:"name"`
	Kind             sources.Kind             `json:"kind"`
	Kubernetes       *kubernetesConnection    `json:"kubernetes,omitempty"`
	KubeSystemUID    string                   `json:"kubeSystemUid,omitempty"`
	ValidationStatus sources.ValidationStatus `json:"validationStatus"`
	LastValidationAt *time.Time               `json:"lastValidationAt,omitempty"`
	CreatedAt        time.Time                `json:"createdAt"`
	UpdatedAt        time.Time                `json:"updatedAt"`
}

func requestOwner(r *http.Request) workspaces.Owner {
	claims := keycloak.ClaimsFromContext(r.Context())
	return workspaces.Owner{
		Issuer:  claims.Issuer,
		Subject: claims.UserID,
	}
}

func workspaceResponse(workspace workspaces.Workspace) workspaceResource {
	return workspaceResource{
		ID:        workspace.ID,
		Name:      workspace.Name,
		CreatedAt: workspace.CreatedAt,
		UpdatedAt: workspace.UpdatedAt,
	}
}

func registrationResponse(registration sources.Registration) registrationResource {
	result := registrationResource{
		ID:               registration.ID,
		WorkspaceID:      registration.WorkspaceID,
		Name:             registration.Name,
		Kind:             registration.Kind,
		ValidationStatus: registration.ValidationStatus,
		LastValidationAt: registration.LastValidationAt,
		CreatedAt:        registration.CreatedAt,
		UpdatedAt:        registration.UpdatedAt,
	}
	if registration.Kubernetes != nil {
		result.Kubernetes = &kubernetesConnection{
			APIEndpoint: registration.Kubernetes.Connection.APIEndpoint,
			Context:     registration.Kubernetes.Connection.Context,
		}
		result.KubeSystemUID = registration.Kubernetes.KubeSystemUID
	}
	return result
}

func connectionParams(connection *kubernetesConnection) *sources.KubernetesConnection {
	if connection == nil {
		return nil
	}
	return &sources.KubernetesConnection{
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
			case errors.Is(err, workspaces.ErrUnauthenticated):
				status = http.StatusUnauthorized
			case errors.Is(err, workspaces.ErrForbidden):
				status = http.StatusForbidden
			case errors.Is(err, sources.ErrNotFound):
				status = http.StatusNotFound
			case errors.Is(err, sources.ErrAlreadyExists), errors.Is(err, workspaces.ErrAlreadyExists):
				status = http.StatusConflict
			case errors.Is(err, sources.ErrInvalidRegistration), errors.Is(err, workspaces.ErrInvalid), errors.Is(err, errInvalidManagementRequest):
				status = http.StatusBadRequest
			}
			writeJSON(w, status, map[string]string{"error": http.StatusText(status)})
		}
	}
}

func registerWorkspaceRoutes(r chi.Router, workspaceService *workspaces.Service, sourceService *sources.Service) {
	r.Get("/workspaces", managementHandler(func(w http.ResponseWriter, r *http.Request) error {
		list, err := workspaceService.List(r.Context(), requestOwner(r))
		if err != nil {
			return fmt.Errorf("list workspaces: %w", err)
		}
		result := make([]workspaceResource, 0, len(list))
		for _, workspace := range list {
			result = append(result, workspaceResponse(workspace))
		}
		writeJSON(w, http.StatusOK, map[string]any{"workspaces": result})
		return nil
	}))
	r.Post("/workspaces", managementHandler(func(w http.ResponseWriter, r *http.Request) error {
		var input struct {
			Name string `json:"name"`
		}
		if err := decodeManagementRequest(w, r, &input); err != nil {
			return fmt.Errorf("decode workspace: %w", err)
		}
		workspace, err := workspaceService.Create(r.Context(), requestOwner(r), input.Name)
		if err != nil {
			return fmt.Errorf("create workspace: %w", err)
		}
		w.Header().Set("Location", "/v1/workspaces/"+workspace.ID.String())
		writeJSON(w, http.StatusCreated, workspaceResponse(workspace))
		return nil
	}))
	r.Route("/workspaces/{workspaceID}", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return managementHandler(func(w http.ResponseWriter, r *http.Request) error {
				if err := workspaceService.Authorize(r.Context(), requestOwner(r), chi.URLParam(r, "workspaceID")); err != nil {
					return fmt.Errorf("authorize workspace: %w", err)
				}
				next.ServeHTTP(w, r)
				return nil
			})
		})
		r.Get("/", managementHandler(func(w http.ResponseWriter, r *http.Request) error {
			workspace, err := workspaceService.Get(r.Context(), requestOwner(r), chi.URLParam(r, "workspaceID"))
			if err != nil {
				return fmt.Errorf("get workspace: %w", err)
			}
			writeJSON(w, http.StatusOK, workspaceResponse(workspace))
			return nil
		}))
		r.Get("/registrations", managementHandler(func(w http.ResponseWriter, r *http.Request) error {
			list, err := sourceService.List(r.Context(), requestOwner(r), chi.URLParam(r, "workspaceID"))
			if err != nil {
				return fmt.Errorf("list registrations: %w", err)
			}
			result := make([]registrationResource, 0, len(list))
			for _, registration := range list {
				result = append(result, registrationResponse(registration))
			}
			writeJSON(w, http.StatusOK, map[string]any{"registrations": result})
			return nil
		}))
		r.Post("/registrations", managementHandler(func(w http.ResponseWriter, r *http.Request) error {
			var input registrationRequest
			if err := decodeManagementRequest(w, r, &input); err != nil {
				return fmt.Errorf("decode registration: %w", err)
			}
			registration, err := sourceService.Create(r.Context(), requestOwner(r), sources.CreateParams{
				WorkspaceID:   chi.URLParam(r, "workspaceID"),
				Name:          input.Name,
				Kind:          input.Kind,
				CredentialRef: input.CredentialRef,
				Kubernetes:    connectionParams(input.Kubernetes),
			})
			if err != nil {
				return fmt.Errorf("create registration: %w", err)
			}
			w.Header().Set("Location", "/v1/workspaces/"+registration.WorkspaceID+"/registrations/"+registration.ID.String())
			writeJSON(w, http.StatusCreated, registrationResponse(registration))
			return nil
		}))
		r.Get("/registrations/{registrationID}", managementHandler(func(w http.ResponseWriter, r *http.Request) error {
			id, err := uuid.Parse(chi.URLParam(r, "registrationID"))
			if err != nil {
				return fmt.Errorf("parse registration ID: %w", errInvalidManagementRequest)
			}
			registration, err := sourceService.Get(r.Context(), requestOwner(r), chi.URLParam(r, "workspaceID"), id)
			if err != nil {
				return fmt.Errorf("get registration: %w", err)
			}
			writeJSON(w, http.StatusOK, registrationResponse(registration))
			return nil
		}))
		r.Put("/registrations/{registrationID}", managementHandler(func(w http.ResponseWriter, r *http.Request) error {
			id, err := uuid.Parse(chi.URLParam(r, "registrationID"))
			if err != nil {
				return fmt.Errorf("parse registration ID: %w", errInvalidManagementRequest)
			}
			var input registrationUpdateRequest
			if err := decodeManagementRequest(w, r, &input); err != nil {
				return fmt.Errorf("decode update: %w", err)
			}
			if err := sourceService.Update(r.Context(), requestOwner(r), chi.URLParam(r, "workspaceID"), id, sources.UpdateParams{
				Name:          input.Name,
				CredentialRef: input.CredentialRef,
				Kubernetes:    connectionParams(input.Kubernetes),
			}); err != nil {
				return fmt.Errorf("update registration: %w", err)
			}
			w.WriteHeader(http.StatusNoContent)
			return nil
		}))
	})
}
