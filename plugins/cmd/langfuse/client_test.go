package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testFrom = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	testTo   = time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
)

// capturingServer serves a fixed body and records the decoded query of every
// request it receives.
func capturingServer(t *testing.T, body string) (*httptest.Server, *[]metricsQuery) {
	t.Helper()

	var seen []metricsQuery
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q metricsQuery
		require.NoError(t, json.Unmarshal([]byte(r.URL.Query().Get("query")), &q))
		seen = append(seen, q)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return server, &seen
}

func testProject(host string) projectConfig {
	return projectConfig{
		ID:        "demo",
		Host:      host,
		UIHost:    host,
		ProjectID: "cm0demo",
		PublicKey: "pk-lf-demo",
		SecretKey: "sk-lf-demo",
		ScoreName: "user_feedback",
		window:    24 * time.Hour,
	}
}

func TestFetchObservationsReadsAggregates(t *testing.T) {
	server, queries := capturingServer(t, `{"data":[{"sum_totalCost":1.2345,"avg_latency":842.5,"count_count":15204}]}`)

	metrics, err := newClient(time.Second).fetchObservations(t.Context(), testProject(server.URL), "", testFrom, testTo)
	require.NoError(t, err)

	assert.Equal(t, "1.2345", metrics.TotalCost)
	assert.Equal(t, "842.5", metrics.AvgLatency)
	assert.Equal(t, "15204", metrics.Count)

	require.Len(t, *queries, 1)
	query := (*queries)[0]
	assert.Equal(t, viewObservations, query.View)
	assert.Empty(t, query.Filters, "an unscoped project measures the whole project")
	assert.Equal(t, "2026-09-23T12:00:00Z", query.FromTimestamp)
	assert.Equal(t, "2026-09-24T12:00:00Z", query.ToTimestamp)
}

func TestFetchObservationsFiltersByTraceName(t *testing.T) {
	server, queries := capturingServer(t, `{"data":[]}`)

	_, err := newClient(time.Second).fetchObservations(t.Context(), testProject(server.URL), "triage-agent", testFrom, testTo)
	require.NoError(t, err)

	require.Len(t, *queries, 1)
	require.Len(t, (*queries)[0].Filters, 1)
	assert.Equal(t, filterSpec{
		Column:   columnTraceName,
		Operator: filterOperatorEqls,
		Value:    "triage-agent",
		Type:     filterTypeString,
	}, (*queries)[0].Filters[0])
}

// An empty window is not a zero-cost window: the difference has to survive to
// the UI, which shows "no data" rather than a confident $0.00.
func TestFetchObservationsReportsNoDataForEmptyResult(t *testing.T) {
	server, _ := capturingServer(t, `{"data":[]}`)

	metrics, err := newClient(time.Second).fetchObservations(t.Context(), testProject(server.URL), "", testFrom, testTo)
	require.NoError(t, err)

	assert.Empty(t, metrics.TotalCost)
	assert.Empty(t, metrics.AvgLatency)
	assert.Empty(t, metrics.Count)
}

func TestFetchObservationsTreatsMissingAndNullKeysAsNoData(t *testing.T) {
	server, _ := capturingServer(t, `{"data":[{"sum_totalCost":null}]}`)

	metrics, err := newClient(time.Second).fetchObservations(t.Context(), testProject(server.URL), "", testFrom, testTo)
	require.NoError(t, err)

	assert.Empty(t, metrics.TotalCost, "a null value means no data")
	assert.Empty(t, metrics.AvgLatency, "an absent key means no data")
}

// Large counts and small costs must not be rounded on their way to a string.
func TestFetchObservationsPreservesPrecision(t *testing.T) {
	server, _ := capturingServer(t, `{"data":[{"sum_totalCost":0.000000123456789,"count_count":9007199254740993}]}`)

	metrics, err := newClient(time.Second).fetchObservations(t.Context(), testProject(server.URL), "", testFrom, testTo)
	require.NoError(t, err)

	assert.Equal(t, "0.000000123456789", metrics.TotalCost)
	assert.Equal(t, "9007199254740993", metrics.Count)
}

func TestFetchScoresFiltersByScoreName(t *testing.T) {
	server, queries := capturingServer(t, `{"data":[{"avg_value":0.82,"count_count":311}]}`)

	metrics, err := newClient(time.Second).fetchScores(t.Context(), testProject(server.URL), "triage-agent", testFrom, testTo)
	require.NoError(t, err)

	assert.Equal(t, "user_feedback", metrics.Name)
	assert.Equal(t, "0.82", metrics.Mean)
	assert.Equal(t, "311", metrics.Count)

	require.Len(t, *queries, 1)
	query := (*queries)[0]
	assert.Equal(t, viewScoresNumeric, query.View)
	assert.Len(t, query.Filters, 2, "scoped score queries filter by trace name and score name")
	assert.Contains(t, query.Filters, filterSpec{
		Column:   columnScoreName,
		Operator: filterOperatorEqls,
		Value:    "user_feedback",
		Type:     filterTypeString,
	})
}

// The secret must travel in the Authorization header, never the URL, so it
// stays out of access logs and proxy traces.
func TestQueryAuthenticatesWithBasicAuthOnly(t *testing.T) {
	var (
		gotUser, gotPass string
		gotOK            bool
		gotRawQuery      string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, gotOK = r.BasicAuth()
		gotRawQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(server.Close)

	_, err := newClient(time.Second).fetchObservations(t.Context(), testProject(server.URL), "", testFrom, testTo)
	require.NoError(t, err)

	assert.True(t, gotOK)
	assert.Equal(t, "pk-lf-demo", gotUser)
	assert.Equal(t, "sk-lf-demo", gotPass)
	assert.NotContains(t, gotRawQuery, "sk-lf-demo")
}

func TestQueryReportsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	_, err := newClient(time.Second).fetchObservations(t.Context(), testProject(server.URL), "", testFrom, testTo)
	assert.ErrorContains(t, err, "401")
}

func TestQueryReportsMalformedBody(t *testing.T) {
	server, _ := capturingServer(t, `{"data":`)

	_, err := newClient(time.Second).fetchObservations(t.Context(), testProject(server.URL), "", testFrom, testTo)
	assert.ErrorContains(t, err, "decoding metrics response")
}

func TestTracesURLJoinsHostAndProject(t *testing.T) {
	assert.Equal(t,
		"https://langfuse.example.com/project/cm0demo/traces",
		tracesURL(testProject("https://langfuse.example.com")),
	)
}

func TestTracesURLKeepsHostBasePath(t *testing.T) {
	assert.Equal(t,
		"https://example.com/project/cm0demo/traces",
		tracesURL(testProject("https://example.com")),
	)
}

func TestResultKeyMatchesAPINaming(t *testing.T) {
	assert.Equal(t, "sum_totalCost", specTotalCost.resultKey())
	assert.Equal(t, "avg_value", specScoreMean.resultKey())
}
