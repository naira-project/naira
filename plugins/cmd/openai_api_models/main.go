// openai plugin that fetches data from openai api spec compatible tools

// # Environment Variables
//
//   - OPENAI_API_MODELS_BASE_URL - MANDATORY - base URL of the endpoint,
//     without the "/v1/models" suffix, e.g.: "https://litellm.example.com" or
//     "http://gateway.example.com:1234/base/"
//
//   - PATH_PREFIX - MANDATORY - prefix for the emitted model Node paths, e.g.
//     "litellm" produces "litellm/gpt-4o". A trailing "/" is optional, so both
//     "litellm" and "litellm/" work.
//
//   - OPENAI_API_MODELS_API_KEY (optional) - bearer token sent to the
//     endpoint; if unset, the request is made unauthenticated.
//
//   - OPENAI_API_MODELS_HTTP_TIMEOUT (optional) - HTTP request timeout;
//     defaults to: 5s.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/naira-project/naira/plugins/internal/openaiutil"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

const propertyKeyOwnedBy = "owned_by"

type config struct {
	PathPrefix  string        `env:"PATH_PREFIX" usage:"prefix for emitted model Node paths, e.g. 'litellm' yields 'litellm/gpt-4o'"`
	BaseURL     string        `env:"OPENAI_API_MODELS_BASE_URL" usage:"base URL of the OpenAI API-compatible endpoint, e.g. 'https://litellm.example.com'"`
	APIKey      string        `env:"OPENAI_API_MODELS_API_KEY"`
	HTTPTimeout time.Duration `env:"OPENAI_API_MODELS_HTTP_TIMEOUT" default:"5s"`
}

type Plugin struct {
	httpClient *http.Client
	logger     *log.Logger
	config     config
	nodePrefix string
}

func New(config config, logger *log.Logger) (*Plugin, error) {
	if strings.TrimSpace(config.BaseURL) == "" {
		return nil, fmt.Errorf("no endpoint configured: OPENAI_API_MODELS_BASE_URL is empty")
	}
	// The prefix is what keeps models of different endpoints apart
	prefix := strings.Trim(strings.TrimSpace(config.PathPrefix), "/")
	if prefix == "" {
		return nil, fmt.Errorf("no node prefix configured: PATH_PREFIX is empty")
	}

	return &Plugin{
		httpClient: &http.Client{Timeout: config.HTTPTimeout},
		logger:     logger,
		config:     config,
		nodePrefix: prefix,
	}, nil
}

func main() {
	app := pluginmain.New[config]()
	p, err := New(app.PluginConfig, app.Logger)
	if err != nil {
		log.Fatalf("failed to initialize plugin: %v", err)
	}
	app.Serve(p)
}

func (p *Plugin) Collect(ctx context.Context) (pluginapi.CollectResponse, error) {
	models, err := openaiutil.FetchModels(ctx, p.httpClient, p.config.BaseURL, p.config.APIKey)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("fetching models from %q: %w", p.config.BaseURL, err)
	}

	nodes := make([]pluginapi.NodeClaim, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			p.logger.Printf("WARN: skipping model with empty id reported by %q", p.config.BaseURL)
			continue
		}
		path := p.nodePrefix + "/" + id
		if _, dup := seen[path]; dup {
			continue
		}
		seen[path] = struct{}{}

		properties := pluginapi.PropertyMap{}
		if model.OwnedBy != "" {
			properties[propertyKeyOwnedBy] = model.OwnedBy
		}

		nodes = append(nodes, pluginapi.NodeClaim{
			ID:         pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: path},
			Properties: properties,
		})
	}

	return pluginapi.CollectResponse{Nodes: nodes, Relations: []pluginapi.RelationClaim{}}, nil
}
