package openmetadata

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/naira-project/naira/catalog/pluginapi"
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

type Plugin struct {
	httpClient *http.Client
	config     Config
}

type Config struct {
	Enabled  bool   `env:"ENABLED" default:"true"`
	BaseURL  string `env:"BASE_URL" default:"http://127.0.0.1:8585"`
	Email    string `env:"ADMIN_EMAIL" default:"admin@open-metadata.org"`
	Password string `env:"ADMIN_PASSWORD" default:"admin"`
}

func New(httpClient *http.Client, config Config) *Plugin {
	return &Plugin{
		httpClient: httpClient,
		config:     config,
	}
}

func (*Plugin) Name() string {
	return pluginName
}

func (p *Plugin) Collect(ctx context.Context) (pluginapi.IngestionRequest, error) {
	token, err := p.login(ctx)
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("authenticating with OpenMetadata: %w", err)
	}

	tables, err := p.fetchTables(ctx, token)
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("fetching OpenMetadata tables: %w", err)
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
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("fetching OpenMetadata lineage: %w", err)
	}

	return pluginapi.IngestionRequest{Nodes: nodes, Relations: relations}, nil
}

func (p *Plugin) collectLineage(ctx context.Context, tables []table, nodeByEntityID map[string]pluginapi.NodeID, token string) ([]pluginapi.RelationClaim, error) {
	relations := make([]pluginapi.RelationClaim, 0)
	seen := make(map[[2]pluginapi.NodeID]struct{})

	for _, table := range tables {
		if table.ID == "" {
			continue
		}

		edges, err := p.fetchTableLineage(ctx, table.ID, token)
		if err != nil {
			return nil, err
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

	return relations, nil
}

func (p *Plugin) fetchTableLineage(ctx context.Context, tableID string, token string) ([]lineageEdge, error) {
	endpoint, err := url.Parse(p.config.BaseURL + "/api/v1/lineage/table/" + url.PathEscape(tableID))
	if err != nil {
		return nil, fmt.Errorf("building OpenMetadata lineage URL: %w", err)
	}

	query := endpoint.Query()
	query.Set("upstreamDepth", "1")
	query.Set("downstreamDepth", "1")
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("building OpenMetadata lineage request: %w", err)
	}
	addAuthorization(req, token)

	var payload lineageResponse
	if err := p.doJSON(req, "lineage", &payload); err != nil {
		return nil, err
	}

	edges := make([]lineageEdge, 0, len(payload.UpstreamEdges)+len(payload.DownstreamEdges))
	edges = append(edges, payload.UpstreamEdges...)
	edges = append(edges, payload.DownstreamEdges...)

	return edges, nil
}

func (p *Plugin) fetchTables(ctx context.Context, token string) ([]table, error) {
	endpoint, err := url.Parse(p.config.BaseURL + "/api/v1/tables")
	if err != nil {
		return nil, fmt.Errorf("building OpenMetadata tables URL: %w", err)
	}

	query := endpoint.Query()
	query.Set("limit", "100")
	query.Set("fields", "columns,tags,owners,service")
	query.Set("include", "non-deleted")
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("building OpenMetadata tables request: %w", err)
	}
	addAuthorization(req, token)

	var payload tablesResponse
	if err := p.doJSON(req, "tables", &payload); err != nil {
		return nil, err
	}

	tables := make([]table, 0, len(payload.Data))
	for _, item := range payload.Data {
		tables = append(tables, table{
			ID:          strings.TrimSpace(item.ID),
			Name:        strings.TrimSpace(item.Name),
			FQN:         strings.TrimSpace(item.FullyQualifiedName),
			Description: strings.TrimSpace(item.Description),
			TableType:   strings.TrimSpace(item.TableType),
			Platform:    strings.TrimSpace(item.Service.Type),
			Columns:     item.Columns,
			Tags:        collectTagFQNs(item.Tags),
			Owners:      collectOwnerNames(item.Owners),
		})
	}

	return tables, nil
}

// login exchanges the configured admin credentials for a short-lived JWT via
// OpenMetadata's basic-auth login endpoint, mirroring the dev seed script. When
// no credentials are configured it returns an empty token, leaving requests
// unauthenticated (for OpenMetadata instances that don't require auth).
func (p *Plugin) login(ctx context.Context) (string, error) {
	if strings.TrimSpace(p.config.Email) == "" && strings.TrimSpace(p.config.Password) == "" {
		return "", nil
	}

	body, _ := json.Marshal(loginRequest{
		Email:    p.config.Email,
		Password: base64.StdEncoding.EncodeToString([]byte(p.config.Password)),
	})

	endpoint := strings.TrimRight(p.config.BaseURL, "/") + "/api/v1/users/login"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("building OpenMetadata login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	var payload loginResponse
	if err := p.doJSON(req, "login", &payload); err != nil {
		return "", err
	}

	token := strings.TrimSpace(payload.AccessToken)
	if token == "" {
		return "", fmt.Errorf("OpenMetadata login response did not contain an access token")
	}

	return token, nil
}

// doJSON sends req, treats any non-2xx status as an error (including a snippet
// of the response body), and decodes a successful JSON response into out. label
// names the call in error messages, e.g. "tables" or "lineage".
func (p *Plugin) doJSON(req *http.Request, label string, out any) error {
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling OpenMetadata %s endpoint: %w", label, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("OpenMetadata %s returned %s: %s", label, resp.Status, strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding OpenMetadata %s response: %w", label, err)
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

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken string `json:"accessToken"`
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
	Service            service   `json:"service"`
	Columns            []column  `json:"columns"`
	Tags               []tagItem `json:"tags"`
	Owners             []owner   `json:"owners"`
}

type service struct {
	Type string `json:"type"`
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
