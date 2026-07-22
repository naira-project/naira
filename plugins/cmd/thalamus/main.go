package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

const (
	pluginName          = "thalamus"
	propertyKeyOwnedBy  = "owned_by"
)

type config struct {
	BaseURL     string        `env:"THALAMUS_BASE_URL" default:"http://127.0.0.1:5000"`
	BearerToken string        `env:"THALAMUS_BEARER_TOKEN"`
	HTTPTimeout time.Duration `env:"THALAMUS_HTTP_TIMEOUT" default:"5s"`
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
	models, err := p.fetchModels(ctx)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("fetching thalamus models: %w", err)
	}

	nodes := make([]pluginapi.NodeClaim, 0, len(models))
	for _, m := range models {
		nodes = append(nodes, pluginapi.NodeClaim{
			ID: pluginapi.NodeID{
				Kind: pluginapi.NodeKindModel,
				Path: pluginName + "/" + m.ID,
			},
			Properties: pluginapi.PropertyMap{
				propertyKeyOwnedBy: m.OwnedBy,
			},
		})
	}

	return pluginapi.CollectResponse{Nodes: nodes, Relations: make([]pluginapi.RelationClaim, 0)}, nil
}

func (p *Plugin) fetchModels(ctx context.Context) ([]model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.config.BaseURL+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("building thalamus models request: %w", err)
	}
	p.addAuthorization(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling thalamus models endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("thalamus models endpoint returned %s", resp.Status)
	}

	var payload modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding thalamus models response: %w", err)
	}

	return payload.Data, nil
}

func (p *Plugin) addAuthorization(req *http.Request) {
	if strings.TrimSpace(p.config.BearerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+p.config.BearerToken)
	}
}

type modelsResponse struct {
	Object string  `json:"object"`
	Data   []model `json:"data"`
}

type model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}
