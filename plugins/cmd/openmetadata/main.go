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

	propertyKeySource      = "source"
	propertyKeyDescription = "description"
	propertyKeyFQN         = "fqn"
	propertyKeySourceURL   = "source_url"
	propertyKeyPlatform    = "platform"
	propertyKeyTableType   = "table_type"
	propertyKeyTags        = "tags"
	propertyKeyOwners      = "owners"
	propertyKeyColumnsJSON = "columns_json"
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

	nodes := make([]pluginapi.NodeClaim, 0, len(tables))
	// nodeByEntityID maps an OpenMetadata entity UUID to the dataset node it
	// produced, so lineage edges (which reference entities by UUID) can be
	// resolved back to nodes that are part of this same ingestion.
	nodeByEntityID := make(map[string]pluginapi.NodeID, len(tables))
	for _, table := range tables {
		properties := pluginapi.PropertyMap{
			propertyKeySource:      pluginName,
			propertyKeyDescription: table.Description,
			propertyKeyFQN:         table.FQN,
			propertyKeySourceURL:   p.tableURL(table.FQN),
		}

		if table.Platform != "" {
			properties[propertyKeyPlatform] = table.Platform
		}
		if table.TableType != "" {
			properties[propertyKeyTableType] = table.TableType
		}
		if len(table.Tags) > 0 {
			properties[propertyKeyTags] = strings.Join(table.Tags, ",")
		}
		if len(table.Owners) > 0 {
			properties[propertyKeyOwners] = strings.Join(table.Owners, ",")
		}
		if columnsJSON := marshalColumns(table.Columns); columnsJSON != "" {
			properties[propertyKeyColumnsJSON] = columnsJSON
		}

		nodeID := pluginapi.NodeID{Kind: pluginapi.NodeKindDataset, Path: datasetPath(table)}
		nodes = append(nodes, pluginapi.NodeClaim{ID: nodeID, Properties: properties})
		if table.ID != "" {
			nodeByEntityID[table.ID] = nodeID
		}
	}

	relations, err := p.collectLineage(ctx, tables, nodeByEntityID, token)
	response := pluginapi.CollectResponse{Nodes: nodes, Relations: relations}
	return response, err
}

// collectLineage fetches lineage for each table; failures for individual
// tables are accumulated and joined into the returned error, so the relations
// gathered from the remaining tables are still ingested.
func (p *Plugin) collectLineage(ctx context.Context, tables []table, nodeByEntityID map[string]pluginapi.NodeID, token string) ([]pluginapi.RelationClaim, error) {
	relations := make([]pluginapi.RelationClaim, 0)
	seen := make(map[[2]pluginapi.NodeID]struct{})
	fetchLineageErrors := make([]error, 0)

	for _, table := range tables {
		if table.ID == "" {
			continue
		}

		edges, err := p.fetchTableLineage(ctx, table.ID, token)
		if err != nil {
			fetchLineageErrors = append(fetchLineageErrors, fmt.Errorf("fetching OpenMetadata lineage for table %s: %w", datasetPath(table), err))
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
func (p *Plugin) fetchTables(ctx context.Context, token string) ([]table, error) {
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

	tables := make([]table, 0, len(payload.Data))
	for _, item := range payload.Data {
		tables = append(tables, table{
			ID:          strings.TrimSpace(item.ID),
			Name:        strings.TrimSpace(item.Name),
			FQN:         strings.TrimSpace(item.FullyQualifiedName),
			Description: strings.TrimSpace(item.Description),
			TableType:   strings.TrimSpace(item.TableType),
			Platform:    strings.TrimSpace(item.ServiceType),
			Columns:     item.Columns,
			Tags:        collectTagFQNs(item.Tags),
			Owners:      collectOwnerNames(item.Owners),
		})
	}

	return tables, nil
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

func datasetPath(table table) string {
	if table.FQN != "" {
		return pluginName + "/" + table.FQN
	}
	if table.Name != "" {
		return pluginName + "/" + table.Name
	}
	return pluginName + "/" + table.ID
}

func marshalColumns(columns []column) string {
	if len(columns) == 0 {
		return ""
	}

	type columnProjection struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		NativeType  string `json:"native_type,omitempty"`
		Description string `json:"description,omitempty"`
	}

	payload := make([]columnProjection, 0, len(columns))
	for _, item := range columns {
		payload = append(payload, columnProjection{
			Name:        strings.TrimSpace(item.Name),
			Type:        strings.TrimSpace(item.DataType),
			NativeType:  strings.TrimSpace(item.DataTypeDisplay),
			Description: strings.TrimSpace(item.Description),
		})
	}

	encoded, err := json.Marshal(payload)
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
	DataTypeDisplay string `json:"dataTypeDisplay"`
	Description     string `json:"description"`
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

type table struct {
	ID          string
	Name        string
	FQN         string
	Description string
	TableType   string
	Platform    string
	Columns     []column
	Tags        []string
	Owners      []string
}
