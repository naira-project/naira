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
	"github.com/naira-project/naira/catalog/internal/sources"
	"github.com/naira-project/naira/catalog/internal/workspaces"
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

func TestWorkspaceRegistrationAPI(t *testing.T) {
	workspaceService := workspaces.NewService(workspaces.NewMemoryStore())
	sourceStore := sources.NewMemoryStore()
	router, err := NewRouter(nil, nil, nil, nil, keycloak.Config{
		Client: workspaceTokenDecoder{},
		Issuer: testIssuer,
	}, workspaceService, sources.NewService(sourceStore, workspaceService))
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
	for _, token := range []string{"", "invalid", "wrong-issuer", "missing-subject"} {
		assert.Equal(t, http.StatusUnauthorized, request(token, http.MethodGet, "/v1/workspaces", "").Code)
	}
	for _, body := range []string{
		`{"name":"production","owner":{"subject":"bob"}}`,
		`{"name":"production","parentId":"parent"}`,
		`{"name":"production"} {"name":"another"}`,
		`{`, `null`, `{"name":""}`,
		`{"name":"` + strings.Repeat("x", 1<<20) + `"}`,
	} {
		assert.Equal(t, http.StatusBadRequest, request("alice", http.MethodPost, "/v1/workspaces", body).Code)
	}
	rec := request("alice", http.MethodPost, "/v1/workspaces", `{"name":"production"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var workspace workspaceResource
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &workspace))
	base := "/v1/workspaces/" + workspace.ID.String()
	assert.Equal(t, base, rec.Header().Get("Location"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.NotContains(t, rec.Body.String(), "owner")
	owner := workspaces.Owner{
		Issuer:  testIssuer,
		Subject: "alice",
	}
	storedWorkspace, err := workspaceService.Get(t.Context(), owner, workspace.ID.String())
	require.NoError(t, err)
	assert.Equal(t, owner, storedWorkspace.Owner)
	assert.Equal(t, http.StatusConflict, request("alice", http.MethodPost, "/v1/workspaces", `{"name":"production"}`).Code)
	assert.Equal(t, http.StatusCreated, request("bob", http.MethodPost, "/v1/workspaces", `{"name":"production"}`).Code)
	other := request("alice", http.MethodPost, "/v1/workspaces", `{"name":"development"}`)
	require.Equal(t, http.StatusCreated, other.Code)
	var otherWorkspace workspaceResource
	require.NoError(t, json.Unmarshal(other.Body.Bytes(), &otherWorkspace))
	var workspaceList struct {
		Workspaces []workspaceResource `json:"workspaces"`
	}
	require.NoError(t, json.Unmarshal(request("alice", http.MethodGet, "/v1/workspaces", "").Body.Bytes(), &workspaceList))
	assert.Equal(t, []workspaceResource{otherWorkspace, workspace}, workspaceList.Workspaces)
	assert.Equal(t, http.StatusOK, request("alice", http.MethodGet, base, "").Code)
	assert.JSONEq(t, `{"registrations":[]}`, request("alice", http.MethodGet, base+"/registrations", "").Body.String())

	const createBody = `{"name":"cluster-a","kind":"kubernetes","credentialRef":"private-reference","kubernetes":{"apiEndpoint":"https://private-endpoint.example.com","context":"production"}}`
	rec = request("alice", http.MethodPost, base+"/registrations", createBody)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var registration registrationResource
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &registration))
	assert.Equal(t, sources.ValidationPending, registration.ValidationStatus)
	assert.Equal(t, workspace.ID.String(), registration.WorkspaceID)
	assert.NotContains(t, rec.Body.String(), "private-reference")
	assert.NotContains(t, rec.Body.String(), "credentialRef")
	path := base + "/registrations/" + registration.ID.String()
	assert.Equal(t, path, rec.Header().Get("Location"))
	assert.Equal(t, http.StatusConflict, request("alice", http.MethodPost, base+"/registrations", createBody).Code)
	assert.Equal(t, http.StatusCreated, request("alice", http.MethodPost, base+"/registrations", strings.Replace(createBody, "cluster-a", "cluster-b", 1)).Code)
	assert.Equal(t, http.StatusCreated, request("alice", http.MethodPost, "/v1/workspaces/"+otherWorkspace.ID.String()+"/registrations", createBody).Code)
	var registrations struct {
		Registrations []registrationResource `json:"registrations"`
	}
	rec = request("alice", http.MethodGet, base+"/registrations", "")
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &registrations))
	require.Len(t, registrations.Registrations, 2)
	assert.NotContains(t, rec.Body.String(), "private-reference")
	assert.NotContains(t, request("alice", http.MethodGet, path, "").Body.String(), "private-reference")

	for _, attempt := range []struct{ method, path, body string }{
		{http.MethodGet, base, ""},
		{http.MethodGet, base + "/registrations", ""},
		{http.MethodPost, base + "/registrations", createBody},
		{http.MethodGet, path, ""},
		{http.MethodPut, path, `{"name":"stolen"}`},
		{http.MethodPut, base + "/registrations/invalid", "malformed JSON"},
		{http.MethodGet, "/v1/workspaces/" + uuid.NewString(), ""},
	} {
		rec := request("bob", attempt.method, attempt.path, attempt.body)
		require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
		assert.JSONEq(t, `{"error":"Forbidden"}`, rec.Body.String())
	}
	assert.Equal(t, http.StatusNotFound, request("alice", http.MethodGet, "/v1/workspaces/"+otherWorkspace.ID.String()+"/registrations/"+registration.ID.String(), "").Code)
	assert.Equal(t, http.StatusBadRequest, request("alice", http.MethodGet, base+"/registrations/invalid", "").Code)
	for _, field := range []string{`"workspaceId":"other"`, `"id":"chosen"`, `"validationStatus":"READY"`, `"kubeSystemUid":"fake"`, `"credential":"raw-secret"`} {
		body := strings.TrimSuffix(createBody, "}") + "," + field + "}"
		assert.Equal(t, http.StatusBadRequest, request("alice", http.MethodPost, base+"/registrations", body).Code)
	}
	const updateBody = `{"name":"renamed","credentialRef":"replacement-reference","kubernetes":{"apiEndpoint":"https://replacement.example.com"}}`
	assert.Equal(t, http.StatusBadRequest, request("alice", http.MethodPut, path, strings.TrimSuffix(updateBody, "}")+`,"kind":"github"}`).Code)
	rec = request("alice", http.MethodPut, path, updateBody)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	stored, err := sourceStore.Get(t.Context(), workspace.ID.String(), registration.ID)
	require.NoError(t, err)
	assert.Equal(t, registration.ID, stored.ID)
	assert.Equal(t, registration.CreatedAt, stored.CreatedAt)
	assert.Equal(t, sources.KindKubernetes, stored.Kind)
	assert.Equal(t, "replacement-reference", stored.CredentialRef)
	assert.Equal(t, sources.ValidationPending, stored.ValidationStatus)
	assert.NotContains(t, request("alice", http.MethodGet, path, "").Body.String(), "replacement-reference")
}
