package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// metricsPath is the Langfuse metrics endpoint. The unversioned
// /api/public/metrics is deprecated and differs in query shape, so the version
// is pinned rather than configurable.
const metricsPath = "/api/public/v2/metrics"

// Views, measures and columns accepted by the metrics API, collected so a
// rename costs one edit. A stale name decodes to an absent metric rather than
// a failed collect; the dev stack's seeder replays these queries against a
// real Langfuse and reports any key that has moved.
const (
	viewObservations   = "observations"
	viewScoresNumeric  = "scores-numeric"
	measureTotalCost   = "totalCost"
	measureLatency     = "latency"
	measureCount       = "count"
	measureValue       = "value"
	aggregationSum     = "sum"
	aggregationAvg     = "avg"
	aggregationCount   = "count"
	columnTraceName    = "traceName"
	columnScoreName    = "name"
	filterTypeString   = "string"
	filterOperatorEqls = "="
)

// metricsQuery is the JSON object the metrics endpoint accepts in its `query`
// parameter.
type metricsQuery struct {
	View          string          `json:"view"`
	Metrics       []metricSpec    `json:"metrics"`
	Dimensions    []dimensionSpec `json:"dimensions"`
	Filters       []filterSpec    `json:"filters"`
	FromTimestamp string          `json:"fromTimestamp"`
	ToTimestamp   string          `json:"toTimestamp"`
}

type metricSpec struct {
	Measure     string `json:"measure"`
	Aggregation string `json:"aggregation"`
}

// resultKey is the column name the API gives this metric in a result row.
func (m metricSpec) resultKey() string { return m.Aggregation + "_" + m.Measure }

type dimensionSpec struct {
	Field string `json:"field"`
}

type filterSpec struct {
	Column   string `json:"column"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
	Type     string `json:"type"`
}

type metricsResponse struct {
	Data []map[string]any `json:"data"`
}

type client struct {
	httpClient *http.Client
}

func newClient(timeout time.Duration) *client {
	return &client{httpClient: &http.Client{Timeout: timeout}}
}

// query runs one metrics query against a project and returns its result rows.
// Queries carry no dimensions in this plugin, so a successful call returns at
// most one row; an empty result means the window held no matching data.
func (c *client) query(ctx context.Context, project projectConfig, q metricsQuery) ([]map[string]any, error) {
	body, err := json.Marshal(q)
	if err != nil {
		return nil, fmt.Errorf("encoding metrics query: %w", err)
	}

	endpoint, err := url.Parse(project.Host + metricsPath)
	if err != nil {
		return nil, fmt.Errorf("building metrics URL: %w", err)
	}
	endpoint.RawQuery = url.Values{"query": {string(body)}}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("building metrics request: %w", err)
	}
	// Basic auth keeps the secret out of the URL, and so out of request logs.
	req.SetBasicAuth(project.PublicKey, project.SecretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling metrics endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("langfuse %s returned %s", metricsPath, resp.Status)
	}

	// UseNumber keeps large counts and small costs exact; the values are
	// re-emitted as strings, so float64 would round them for no gain.
	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()

	var payload metricsResponse
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding metrics response: %w", err)
	}

	return payload.Data, nil
}

// observationMetrics is the cost and latency slice of one scope's window.
type observationMetrics struct {
	TotalCost  string
	AvgLatency string
	Count      string
}

// scoreMetrics is the aggregate user feedback for one scope's window.
type scoreMetrics struct {
	Name  string
	Mean  string
	Count string
}

var (
	specTotalCost  = metricSpec{Measure: measureTotalCost, Aggregation: aggregationSum}
	specAvgLatency = metricSpec{Measure: measureLatency, Aggregation: aggregationAvg}
	specCount      = metricSpec{Measure: measureCount, Aggregation: aggregationCount}
	specScoreMean  = metricSpec{Measure: measureValue, Aggregation: aggregationAvg}
	specScoreCount = metricSpec{Measure: measureCount, Aggregation: aggregationCount}
)

// fetchObservations aggregates cost, latency and volume over the window for
// one scope. An empty traceName measures the whole project.
func (c *client) fetchObservations(ctx context.Context, project projectConfig, traceName string, from, to time.Time) (observationMetrics, error) {
	rows, err := c.query(ctx, project, metricsQuery{
		View:          viewObservations,
		Metrics:       []metricSpec{specTotalCost, specAvgLatency, specCount},
		Dimensions:    []dimensionSpec{},
		Filters:       traceNameFilters(traceName),
		FromTimestamp: formatTimestamp(from),
		ToTimestamp:   formatTimestamp(to),
	})
	if err != nil {
		return observationMetrics{}, fmt.Errorf("querying observation metrics: %w", err)
	}

	row := firstRow(rows)
	return observationMetrics{
		TotalCost:  numericValue(row, specTotalCost.resultKey()),
		AvgLatency: numericValue(row, specAvgLatency.resultKey()),
		Count:      numericValue(row, specCount.resultKey()),
	}, nil
}

// fetchScores aggregates one numeric score over the window for one scope.
// Projects that declare no score_name are skipped by the caller.
func (c *client) fetchScores(ctx context.Context, project projectConfig, traceName string, from, to time.Time) (scoreMetrics, error) {
	filters := append(traceNameFilters(traceName), filterSpec{
		Column:   columnScoreName,
		Operator: filterOperatorEqls,
		Value:    project.ScoreName,
		Type:     filterTypeString,
	})

	rows, err := c.query(ctx, project, metricsQuery{
		View:          viewScoresNumeric,
		Metrics:       []metricSpec{specScoreMean, specScoreCount},
		Dimensions:    []dimensionSpec{},
		Filters:       filters,
		FromTimestamp: formatTimestamp(from),
		ToTimestamp:   formatTimestamp(to),
	})
	if err != nil {
		return scoreMetrics{}, fmt.Errorf("querying score metrics: %w", err)
	}

	row := firstRow(rows)
	return scoreMetrics{
		Name:  project.ScoreName,
		Mean:  numericValue(row, specScoreMean.resultKey()),
		Count: numericValue(row, specScoreCount.resultKey()),
	}, nil
}

func traceNameFilters(traceName string) []filterSpec {
	if traceName == "" {
		return []filterSpec{}
	}
	return []filterSpec{{
		Column:   columnTraceName,
		Operator: filterOperatorEqls,
		Value:    traceName,
		Type:     filterTypeString,
	}}
}

func firstRow(rows []map[string]any) map[string]any {
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}

// formatTimestamp renders an instant the way the metrics API expects it.
func formatTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// numericValue reads one metric from a result row as a decimal string. An
// absent row, key or null gives "", meaning "no data" rather than zero — the
// UI must not show a confident $0.00 for a window it never measured.
func numericValue(row map[string]any, key string) string {
	raw, ok := row[key]
	if !ok || raw == nil {
		return ""
	}

	switch value := raw.(type) {
	case json.Number:
		return value.String()
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case string:
		// Tolerated so a numeric-as-string response lands, but only if it
		// really is a number; anything else is a shape change, so "no data".
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return ""
		}
		return value
	default:
		return ""
	}
}

// tracesURL is the deep link to this scope's traces in the Langfuse UI. Built
// from UIHost, not Host: a browser follows it and may not reach the address
// the plugin calls the API on.
//
// TODO(langfuse): the trace-list filter encoding is version-specific, so a
// scoped link lands on the unfiltered trace list. Add the traceName filter
// here once pinned — the plugin owns this URL so the UI need not know
// Langfuse's routing.
func tracesURL(project projectConfig) string {
	base := &url.URL{Path: "/project/" + project.ProjectID + "/traces"}

	host, err := url.Parse(project.UIHost)
	if err != nil {
		return base.String()
	}
	return host.ResolveReference(base).String()
}
