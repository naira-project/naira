package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

const (
	pluginName = "openmetadata"

	propertyKeySource             = "source"
	propertyKeyDescription        = "description"
	propertyKeyFullyQualifiedName = "fullyQualifiedName"
	propertyKeySourceURL          = "source_url"
	propertyKeyServiceType        = "serviceType"
	propertyKeyTableType          = "tableType"
	propertyKeyTags               = "tags"
	propertyKeyOwners             = "owners"
	propertyKeyColumns            = "columns"
)

type config struct {
	BaseURL     string        `env:"OPENMETADATA_BASE_URL" default:"http://127.0.0.1:8585"`
	Email       string        `env:"OPENMETADATA_ADMIN_EMAIL"`
	Password    string        `env:"OPENMETADATA_ADMIN_PASSWORD"`
	HTTPTimeout time.Duration `env:"OPENMETADATA_HTTP_TIMEOUT" default:"5s"`
}

type Plugin struct {
	httpClient *http.Client
	config     config
}

func New(config config) *Plugin {
	return &Plugin{
		httpClient: &http.Client{Timeout: config.HTTPTimeout},
		config:     config,
	}
}

func main() {
	app := pluginmain.New[config]()

	p := New(app.PluginConfig)

	app.Serve(p)
}

func (p *Plugin) Collect(ctx context.Context) (pluginapi.CollectResponse, error) {
	token, err := p.login(ctx)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("authenticating with OpenMetadata: %w", err)
	}

	tables, err := p.fetchTables(ctx, token)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("fetching OpenMetadata tables: %w", err)
	}

	nodes, nodeByEntityID := p.collectNodes(tables)

	relations, err := p.collectLineage(ctx, tables, nodeByEntityID, token)
	response := pluginapi.CollectResponse{Nodes: nodes, Relations: relations}
	if err != nil {
		return response, fmt.Errorf("collecting lineage: %w", err)
	}

	return response, nil
}

// collectNodes converts tables into dataset node claims. It also returns a map
// from OpenMetadata entity UUID to the node it produced, so lineage edges
// (which reference entities by UUID) can be resolved back to the nodes
func (p *Plugin) collectNodes(tables []tableItem) ([]pluginapi.NodeClaim, map[string]pluginapi.NodeID) {
	nodes := make([]pluginapi.NodeClaim, 0, len(tables))
	nodeByEntityID := make(map[string]pluginapi.NodeID, len(tables))

	for _, table := range tables {
		if table.ID == "" {
			continue
		}

		properties := pluginapi.PropertyMap{
			propertyKeySource:             pluginName,
			propertyKeyDescription:        table.Description,
			propertyKeyFullyQualifiedName: table.FullyQualifiedName,
			propertyKeySourceURL:          p.tableURL(table.FullyQualifiedName),
		}

		if table.ServiceType != "" {
			properties[propertyKeyServiceType] = table.ServiceType
		}
		if table.TableType != "" {
			properties[propertyKeyTableType] = table.TableType
		}
		if tags := collectTagFQNs(table.Tags); len(tags) > 0 {
			properties[propertyKeyTags] = strings.Join(tags, ",")
		}
		if owners := collectOwnerNames(table.Owners); len(owners) > 0 {
			properties[propertyKeyOwners] = strings.Join(owners, ",")
		}
		if columnsJSON := marshalColumns(table.Columns); columnsJSON != "" {
			properties[propertyKeyColumns] = columnsJSON
		}

		nodeID := pluginapi.NodeID{Kind: pluginapi.NodeKindDataset, Path: pluginName + "/" + table.ID}
		nodes = append(nodes, pluginapi.NodeClaim{ID: nodeID, Properties: properties})
		nodeByEntityID[table.ID] = nodeID
	}

	return nodes, nodeByEntityID
}

// collectLineage fetches lineage for each table; failures for individual
// tables are accumulated and joined into the returned error, so the relations
// gathered from the remaining tables are still ingested.
func (p *Plugin) collectLineage(ctx context.Context, tables []tableItem, nodeByEntityID map[string]pluginapi.NodeID, token string) ([]pluginapi.RelationClaim, error) {
	relations := make([]pluginapi.RelationClaim, 0)
	seen := make(map[[2]pluginapi.NodeID]struct{})
	fetchLineageErrors := make([]error, 0)

	for _, table := range tables {
		if table.ID == "" {
			continue
		}

		edges, err := p.fetchTableLineage(ctx, table.ID, token)
		if err != nil {
			fetchLineageErrors = append(fetchLineageErrors, fmt.Errorf("fetching OpenMetadata lineage for table %s: %w", table.ID, err))
			continue
		}

		for _, edge := range edges {
			upstream, ok := nodeByEntityID[strings.TrimSpace(edge.FromEntity)]
			if !ok {
				continue
			}
			downstream, ok := nodeByEntityID[strings.TrimSpace(edge.ToEntity)]
			if !ok {
				continue
			}

			key := [2]pluginapi.NodeID{downstream, upstream}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}

			relations = append(relations, pluginapi.RelationClaim{
				Kind: pluginapi.RelationKindDerivedFrom,
				From: downstream,
				To:   upstream,
			})
		}
	}

	if len(fetchLineageErrors) > 0 {
		return relations, errors.Join(fetchLineageErrors...)
	}

	return relations, nil
}

func (p *Plugin) fetchTableLineage(ctx context.Context, tableID string, token string) ([]lineageEdge, error) {
	endpoint, err := url.Parse(p.config.BaseURL + "/api/v1/lineage/table/" + url.PathEscape(tableID))
	if err != nil {
		return nil, fmt.Errorf("building API URL: %w", err)
	}

	query := endpoint.Query()
	query.Set("upstreamDepth", "1")
	query.Set("downstreamDepth", "1")
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	addAuthorization(req, token)

	var payload lineageResponse
	if err := p.doJSON(req, &payload); err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	edges := make([]lineageEdge, 0, len(payload.UpstreamEdges)+len(payload.DownstreamEdges))
	edges = append(edges, payload.UpstreamEdges...)
	edges = append(edges, payload.DownstreamEdges...)

	return edges, nil
}

// TODO: add pagination; tables beyond the first page are currently dropped.
func (p *Plugin) fetchTables(ctx context.Context, token string) ([]tableItem, error) {
	endpoint, err := url.Parse(p.config.BaseURL + "/api/v1/tables")
	if err != nil {
		return nil, fmt.Errorf("building API URL: %w", err)
	}

	query := endpoint.Query()
	query.Set("limit", "100")
	query.Set("fields", "columns,tags,owners")
	query.Set("include", "non-deleted")
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	addAuthorization(req, token)

	var payload tablesResponse
	if err := p.doJSON(req, &payload); err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	return payload.Data, nil
}

// login exchanges the configured admin credentials for a short-lived JWT via
// OpenMetadata's basic-auth login endpoint. When no credentials are configured
// it returns an empty token, leaving requests unauthenticated (for
// OpenMetadata instances that don't require auth).
func (p *Plugin) login(ctx context.Context) (string, error) {
	email := strings.TrimSpace(p.config.Email)
	password := strings.TrimSpace(p.config.Password)
	if email == "" && password == "" {
		return "", nil
	}
	if email == "" || password == "" {
		return "", errors.New("both OPENMETADATA_ADMIN_EMAIL and OPENMETADATA_ADMIN_PASSWORD must be set, or neither")
	}

	body, err := json.Marshal(map[string]string{
		"email":    email,
		"password": base64.StdEncoding.EncodeToString([]byte(password)),
	})
	if err != nil {
		return "", fmt.Errorf("encoding request: %w", err)
	}

	endpoint := strings.TrimRight(p.config.BaseURL, "/") + "/api/v1/users/login"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	var loginResponse struct {
		AccessToken string `json:"accessToken"`
	}
	if err := p.doJSON(req, &loginResponse); err != nil {
		return "", fmt.Errorf("executing request: %w", err)
	}

	token := strings.TrimSpace(loginResponse.AccessToken)
	if token == "" {
		return "", errors.New("response did not contain an access token")
	}

	return token, nil
}

// doJSON sends req, treats any non-2xx status as an error (including a snippet
// of the response body), and decodes a successful JSON response into out.
func (p *Plugin) doJSON(req *http.Request, out any) error {
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("got HTTP status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	return nil
}

func addAuthorization(req *http.Request, token string) {
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func (p *Plugin) tableURL(fqn string) string {
	if strings.TrimSpace(fqn) == "" {
		return ""
	}

	return strings.TrimRight(p.config.BaseURL, "/") + "/table/" + url.PathEscape(fqn)
}

func marshalColumns(columns []column) string {
	if len(columns) == 0 {
		return ""
	}

	encoded, err := json.Marshal(columns)
	if err != nil {
		return ""
	}

	return string(encoded)
}

func collectTagFQNs(tags []tagItem) []string {
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tagName := strings.TrimSpace(tag.TagFQN); tagName != "" {
			result = append(result, tagName)
		}
	}
	return result
}

func collectOwnerNames(owners []owner) []string {
	result := make([]string, 0, len(owners))
	for _, item := range owners {
		if name := strings.TrimSpace(item.Name); name != "" {
			result = append(result, name)
		}
	}
	return result
}

type tablesResponse struct {
	Data []tableItem `json:"data"`
}

type tableItem struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	FullyQualifiedName string    `json:"fullyQualifiedName"`
	Description        string    `json:"description"`
	TableType          string    `json:"tableType"`
	ServiceType        string    `json:"serviceType"`
	Columns            []column  `json:"columns"`
	Tags               []tagItem `json:"tags"`
	Owners             []owner   `json:"owners"`
}

type column struct {
	Name            string `json:"name"`
	DataType        string `json:"dataType"`
	DataTypeDisplay string `json:"dataTypeDisplay,omitempty"`
	Description     string `json:"description,omitempty"`
}

type tagItem struct {
	TagFQN string `json:"tagFQN"`
}

type owner struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type lineageResponse struct {
	UpstreamEdges   []lineageEdge `json:"upstreamEdges"`
	DownstreamEdges []lineageEdge `json:"downstreamEdges"`
}

type lineageEdge struct {
	FromEntity string `json:"fromEntity"`
	ToEntity   string `json:"toEntity"`
}
