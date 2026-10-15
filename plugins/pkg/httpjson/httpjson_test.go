package httpjson

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDo_SuccessDecodesBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/widgets", r.URL.Path)
		fmt.Fprint(w, `{"name":"gizmo"}`)
	}))
	defer srv.Close()

	var out struct {
		Name string `json:"name"`
	}
	err := Do(t.Context(), srv.Client(), http.MethodGet, srv.URL+"/widgets", nil, &out)
	require.NoError(t, err)
	assert.Equal(t, "gizmo", out.Name)
}

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Empty(t, r.Header.Get("Content-Type"))
		fmt.Fprint(w, `{"name":"gizmo"}`)
	}))
	defer srv.Close()

	var out struct {
		Name string `json:"name"`
	}
	err := Get(t.Context(), srv.Client(), srv.URL, &out)
	require.NoError(t, err)
	assert.Equal(t, "gizmo", out.Name)
}

func TestPost(t *testing.T) {
	var gotContentType, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	var out struct {
		OK bool `json:"ok"`
	}
	err := Post(t.Context(), srv.Client(), srv.URL, map[string]string{"a": "b"}, &out)
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "application/json", gotContentType)
	assert.True(t, out.OK)
}

func TestDo_HttpStatusAcceptance(t *testing.T) {
	tests := []struct {
		name          string
		status        int
		body          string
		wantErrStatus *int
	}{
		{
			name:   "200 OK is accepted",
			status: http.StatusOK,
			body:   `{}`,
		},
		{
			name:   "201 Created is accepted",
			status: http.StatusCreated,
			body:   `{}`,
		},
		{
			name:   "299 edge of 2xx range is accepted",
			status: 299,
			body:   `{}`,
		},
		{
			name:          "500 Internal Server Error is rejected",
			status:        http.StatusInternalServerError,
			body:          "boom",
			wantErrStatus: new(http.StatusInternalServerError),
		},
		{
			name:          "300 Multiple Choices is rejected",
			status:        http.StatusMultipleChoices,
			body:          "multiple choices redirect",
			wantErrStatus: new(http.StatusMultipleChoices),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer srv.Close()

			var out map[string]any
			err := Do(t.Context(), srv.Client(), http.MethodGet, srv.URL, nil, &out)

			if tt.wantErrStatus != nil {
				require.Error(t, err)
				var statusErr *StatusAcceptError
				require.ErrorAs(t, err, &statusErr)
				assert.Equal(t, *tt.wantErrStatus, statusErr.StatusCode)
				assert.Equal(t, tt.body, statusErr.Body)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDo_WithAcceptStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	isHTTPOK := func(code int) bool { return code == http.StatusOK }
	err := Do(t.Context(), srv.Client(), http.MethodGet, srv.URL, nil, nil,
		WithAcceptStatus(isHTTPOK))
	require.Error(t, err)

	var statusErr *StatusAcceptError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusCreated, statusErr.StatusCode)
}

func TestGet_WithAuthorizationBearer(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		wantAuth []string
	}{
		{
			name:     "non-empty token",
			token:    "secret",
			wantAuth: []string{"Bearer secret"},
		},
		{
			name:     "blank token",
			token:    "   ",
			wantAuth: nil,
		},
		{
			name:     "empty token",
			token:    "",
			wantAuth: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAuth []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header["Authorization"]
				fmt.Fprint(w, `{}`)
			}))
			defer srv.Close()

			err := Get(t.Context(), srv.Client(), srv.URL, nil,
				WithAuthorizationBearer(tt.token))
			require.NoError(t, err)
			assert.Equal(t, tt.wantAuth, gotAuth)
		})
	}
}

func TestBuildURL(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		params  url.Values
		want    string
		wantErr bool
	}{
		{
			name: "add params to simple base URL",
			base: "https://example.com/widgets",
			params: url.Values{
				"limit":  {"100"},
				"fields": {"a,b"},
			},
			want: "https://example.com/widgets?fields=a%2Cb&limit=100",
		},
		{
			name: "overwrite existing query param",
			base: "https://example.com/widgets?limit=10",
			params: url.Values{
				"limit": {"9"},
			},
			want: "https://example.com/widgets?limit=9",
		},
		{
			name: "multi-valued param",
			base: "https://example.com/widgets",
			params: url.Values{
				"tag": {"a", "b"},
			},
			want: "https://example.com/widgets?tag=a&tag=b",
		},
		{
			name:    "invalid base URL returns error",
			base:    ":not-a-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildURL(tt.base, tt.params)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.String())
		})
	}
}

func TestDo_WithHeaders_OverrideDefaultAcceptHeader(t *testing.T) {
	var gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	err := Do(t.Context(), srv.Client(), http.MethodGet, srv.URL, nil, nil,
		WithHeaders{"Accept": "application/vnd.github+json"})
	require.NoError(t, err)
	assert.Equal(t, "application/vnd.github+json", gotAccept)
}

func TestDo_WithMaxResponseBytes(t *testing.T) {
	bigValue := strings.Repeat("A", 1024)
	rawJSON := fmt.Sprintf(`{"value":"%s"}`, bigValue)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(rawJSON))
	}))
	defer srv.Close()

	type response struct {
		Value string `json:"value"`
	}

	// Tested without a limit, the JSON decodes successfully
	out := response{}
	assert.NoError(t,
		Do(t.Context(), srv.Client(), http.MethodGet, srv.URL, nil, &out))
	assert.Equal(t, bigValue, out.Value)

	// With a limit of 16 bytes, an error is triggered even though the same JSON decoded successfully above.
	out = response{} // reset
	assert.Error(t,
		Do(t.Context(), srv.Client(), http.MethodGet, srv.URL, nil, &out,
			WithMaxResponseBytes(16)))
	assert.Equal(t, "", out.Value)
}

func TestDo_PayloadEncodedAsJSON(t *testing.T) {
	var gotContentType, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		buf, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusInternalServerError)
			return
		}
		gotBody = string(buf)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	payload := map[string]string{"email": "a@example.com"}
	var out struct {
		OK bool `json:"ok"`
	}
	err := Do(t.Context(), srv.Client(), http.MethodPost, srv.URL, payload, &out)
	require.NoError(t, err)
	assert.Equal(t, "application/json", gotContentType)
	assert.JSONEq(t, `{"email":"a@example.com"}`, gotBody)
	assert.True(t, out.OK)
}

func TestDo_NilOutSkipsDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `not valid json`)
	}))
	defer srv.Close()

	err := Do(t.Context(), srv.Client(), http.MethodGet, srv.URL, nil, nil)
	require.NoError(t, err)
}

func TestDo_MalformedJSONResponseReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `not valid json`)
	}))
	defer srv.Close()

	var out map[string]any
	err := Do(t.Context(), srv.Client(), http.MethodGet, srv.URL, nil, &out)
	require.Error(t, err)

	var statusErr *StatusAcceptError
	assert.NotErrorAs(t, err, &statusErr)
	var jsonErr *json.SyntaxError
	assert.ErrorAs(t, err, &jsonErr)
}
