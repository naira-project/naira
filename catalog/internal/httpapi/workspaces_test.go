package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/naira-project/naira/catalog/internal/auth/keycloak"
	"github.com/naira-project/naira/catalog/internal/clusterregistry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type workspaceTokenDecoder struct{}

func (workspaceTokenDecoder) DecodeAccessToken(_ context.Context, token, _ string) (*jwt.Token, *jwt.MapClaims, error) {
	if token != "alice" && token != "bob" && token != "wrong-issuer" && token != "missing-subject" {
		return nil, nil, errors.New("invalid token")
	}
	claims := jwt.MapClaims{"sub": token, "iss": testIssuer}
	if token == "wrong-issuer" {
		claims["iss"] = "https://other.example"
	}
	if token == "missing-subject" {
		delete(claims, "sub")
	}
	return nil, &claims, nil
}

func TestFixedWorkspaceRegistrationAPI(t *testing.T) {
	clusterRegistry := newTestClusterRegistry(t)
	router, err := NewRouter(nil, nil, nil, nil, keycloak.Config{
		Client: workspaceTokenDecoder{},
		Issuer: testIssuer,
	}, clusterRegistry)
	require.NoError(t, err)
	request := func(token, method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	const base = "/v1/workspaces/test-workspace"
	const createBody = `{"name":"cluster-a","credentialRef":"private-reference","kubernetes":{"apiEndpoint":"https://cluster.example.com","context":"production"}}`
	for _, token := range []string{"", "invalid", "wrong-issuer", "missing-subject"} {
		for _, attempt := range []struct {
			method string
			path   string
		}{
			{method: http.MethodGet, path: "/v1/workspaces"},
			{method: http.MethodGet, path: base},
			{method: http.MethodGet, path: base + "/registrations"},
			{method: http.MethodPost, path: base + "/registrations"},
			{method: http.MethodGet, path: base + "/registrations/" + uuid.NewString()},
			{method: http.MethodPut, path: base + "/registrations/" + uuid.NewString()},
		} {
			assert.Equal(t, http.StatusUnauthorized, request(token, attempt.method, attempt.path, createBody).Code)
		}
	}
	for _, token := range []string{"alice", "bob"} {
		rec := request(token, http.MethodGet, "/v1/workspaces", "")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"workspaces":[{"id":"test-workspace","name":"test-workspace"}]}`, rec.Body.String())
		assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		rec = request(token, http.MethodGet, base, "")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"id":"test-workspace","name":"test-workspace"}`, rec.Body.String())
	}
	for _, path := range []string{"/v1/workspaces", base} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			rec := request("alice", method, path, `{"name":"renamed"}`)
			assert.Equal(t, http.StatusMethodNotAllowed, rec.Code, "%s %s", method, path)
		}
	}
	assert.JSONEq(t, `{"registrations":[]}`, request("alice", http.MethodGet, base+"/registrations", "").Body.String())

	for _, body := range []string{
		`{`, `null`, `{}`, createBody + ` {}`,
		`{"name":"cluster-a","credentialRef":"private-reference"}`,
		`{"name":"cluster-a","credentialRef":"private-reference","kubernetes":null}`,
		`{"name":"cluster-a","credentialRef":"private-reference","kubernetes":{}}`,
		`{"name":"` + strings.Repeat("x", 1<<20) + `"}`,
	} {
		assert.Equal(t, http.StatusBadRequest, request("alice", http.MethodPost, base+"/registrations", body).Code)
	}
	for _, field := range []string{`"kind":"kubernetes"`, `"kind":"github"`, `"workspaceId":"other"`, `"owner":"bob"`, `"id":"chosen"`, `"validationStatus":"READY"`, `"kubeSystemUid":"fake"`, `"credential":"raw-secret"`} {
		body := strings.TrimSuffix(createBody, "}") + "," + field + "}"
		assert.Equal(t, http.StatusBadRequest, request("alice", http.MethodPost, base+"/registrations", body).Code)
	}
	rec := request("alice", http.MethodPost, base+"/registrations", createBody)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var registration registrationResource
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &registration))
	assert.Equal(t, "test-workspace", registration.WorkspaceID)
	assert.Equal(t, clusterregistry.ValidationPending, registration.ValidationStatus)
	assert.NotContains(t, rec.Body.String(), "private-reference")
	assert.NotContains(t, rec.Body.String(), "credentialRef")
	assert.NotContains(t, rec.Body.String(), `"kind"`)
	path := base + "/registrations/" + registration.ID.String()
	assert.Equal(t, path, rec.Header().Get("Location"))
	assert.Equal(t, http.StatusConflict, request("bob", http.MethodPost, base+"/registrations", createBody).Code)
	assert.Equal(t, http.StatusCreated, request("bob", http.MethodPost, base+"/registrations", strings.Replace(createBody, "cluster-a", "cluster-b", 1)).Code)
	for _, token := range []string{"alice", "bob"} {
		rec = request(token, http.MethodGet, base+"/registrations", "")
		require.Equal(t, http.StatusOK, rec.Code)
		var list struct {
			Registrations []registrationResource `json:"registrations"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
		require.Len(t, list.Registrations, 2)
		assert.NotContains(t, rec.Body.String(), "private-reference")
		rec = request(token, http.MethodGet, path, "")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.NotContains(t, rec.Body.String(), "private-reference")
	}

	for _, other := range []string{"other-workspace", "test-workspace-extra", "TEST-WORKSPACE", uuid.NewString()} {
		otherBase := "/v1/workspaces/" + other
		for _, attempt := range []struct {
			method string
			path   string
		}{
			{method: http.MethodGet, path: otherBase},
			{method: http.MethodGet, path: otherBase + "/registrations"},
			{method: http.MethodPost, path: otherBase + "/registrations"},
			{method: http.MethodGet, path: otherBase + "/registrations/" + registration.ID.String()},
			{method: http.MethodPut, path: otherBase + "/registrations/invalid"},
		} {
			rec = request("alice", attempt.method, attempt.path, "malformed JSON")
			require.Equal(t, http.StatusNotFound, rec.Code)
			assert.JSONEq(t, `{"error":"Not Found"}`, rec.Body.String())
		}
	}
	assert.Equal(t, http.StatusNotFound, request("alice", http.MethodGet, base+"/registrations/"+uuid.NewString(), "").Code)
	assert.Equal(t, http.StatusBadRequest, request("alice", http.MethodGet, base+"/registrations/invalid", "").Code)
	const updateBody = `{"name":"renamed","credentialRef":"replacement-reference","kubernetes":{"apiEndpoint":"https://replacement.example.com"}}`
	for _, field := range []string{`"kind":"github"`, `"workspaceId":"other"`} {
		assert.Equal(t, http.StatusBadRequest, request("bob", http.MethodPut, path, strings.TrimSuffix(updateBody, "}")+","+field+"}").Code)
	}
	for _, invalidConnection := range []string{
		`{"name":"renamed","credentialRef":"replacement-reference"}`,
		`{"name":"renamed","credentialRef":"replacement-reference","kubernetes":null}`,
		`{"name":"renamed","credentialRef":"replacement-reference","kubernetes":{}}`,
	} {
		assert.Equal(t, http.StatusBadRequest, request("bob", http.MethodPut, path, invalidConnection).Code)
		unchanged, err := clusterRegistry.Registrations.Get(t.Context(), clusterregistry.Caller{Issuer: testIssuer, Subject: "bob"}, clusterregistry.Workspace{ID: "test-workspace"}, registration.ID)
		require.NoError(t, err)
		assert.Equal(t, registration, registrationResponse(unchanged))
		assert.Equal(t, "private-reference", unchanged.CredentialRef)
	}
	rec = request("bob", http.MethodPut, path, updateBody)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	stored, err := clusterRegistry.Registrations.Get(t.Context(), clusterregistry.Caller{Issuer: testIssuer, Subject: "bob"}, clusterregistry.Workspace{ID: "test-workspace"}, registration.ID)
	require.NoError(t, err)
	assert.Equal(t, registration.ID, stored.ID)
	assert.Equal(t, "test-workspace", stored.Workspace.ID)
	assert.Equal(t, registration.CreatedAt, stored.CreatedAt)
	assert.Equal(t, "renamed", stored.Name)
	assert.Equal(t, "replacement-reference", stored.CredentialRef)
	rec = request("alice", http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "renamed")
	assert.NotContains(t, rec.Body.String(), "replacement-reference")
	assert.JSONEq(t, `{"workspaces":[{"id":"test-workspace","name":"test-workspace"}]}`, request("alice", http.MethodGet, "/v1/workspaces", "").Body.String())
}

func TestWorkspaceRoutesCheckAccessBeforeParsing(t *testing.T) {
	router, err := NewRouter(nil, nil, nil, nil, keycloak.Config{
		Client: workspaceTokenDecoder{},
		Issuer: testIssuer,
	}, newTestClusterRegistry(t))
	require.NoError(t, err)

	for _, tt := range []struct {
		name   string
		method string
		path   string
		token  string
		want   int
	}{
		{
			name:   "workspace with trailing slash",
			method: http.MethodGet,
			path:   "/v1/workspaces/test-workspace/",
			token:  "alice",
			want:   http.StatusOK,
		},
		{
			name:   "authentication before workspace lookup",
			method: http.MethodGet,
			path:   "/v1/workspaces/unknown",
			want:   http.StatusUnauthorized,
		},
		{
			name:   "authentication before test ID parsing",
			method: http.MethodPost,
			path:   "/v1/workspaces/test-workspace/registrations/invalid:test",
			want:   http.StatusUnauthorized,
		},
		{
			name:   "workspace before create body parsing",
			method: http.MethodPost,
			path:   "/v1/workspaces/unknown/registrations",
			token:  "alice",
			want:   http.StatusNotFound,
		},
		{
			name:   "workspace before read ID parsing",
			method: http.MethodGet,
			path:   "/v1/workspaces/unknown/registrations/invalid",
			token:  "alice",
			want:   http.StatusNotFound,
		},
		{
			name:   "workspace before update parsing",
			method: http.MethodPut,
			path:   "/v1/workspaces/unknown/registrations/invalid",
			token:  "alice",
			want:   http.StatusNotFound,
		},
		{
			name:   "workspace before test ID parsing",
			method: http.MethodPost,
			path:   "/v1/workspaces/unknown/registrations/invalid:test",
			token:  "alice",
			want:   http.StatusNotFound,
		},
		{
			name:   "test parses ID after access check",
			method: http.MethodPost,
			path:   "/v1/workspaces/test-workspace/registrations/invalid:test",
			token:  "alice",
			want:   http.StatusBadRequest,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader("invalid JSON"))
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			assert.Equal(t, tt.want, rec.Code, rec.Body.String())
			if tt.token != "" {
				assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
			}
		})
	}
}
