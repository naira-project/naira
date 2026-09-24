// openai plugin that fetches data from openai api spec compatible tools

// # Environment Variables
//
//   - OPENAI_API_MODELS_BASE_URL - MANDATORY - base URL of the endpoint,
//     without the "/v1/models" suffix, e.g.: "https://litellm.example.com" or
//     "http://gateway.example.com:1234/base/"
//
//   - PATH_PREFIX - MANDATORY - prefix for the emitted model Node paths, e.g.
//     "litellm" produces "litellm/gpt-4o".
//
//   - OPENAI_API_MODELS_API_KEY (optional) - bearer token sent to the
//     endpoint; if unset, the request is made unauthenticated.
//
//   - OPENAI_API_MODELS_HTTP_TIMEOUT (optional) - HTTP request timeout;
//     defaults to: 5s.
package main

import (
	"context"
	"encoding/json"
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

// datum embeds openaiutil.Datum and additionally captures every field of a
// /v1/models entry that isn't one of Datum's well-known fields, so that
// provider-specific extras (e.g. llama.cpp's "meta") surface as node
// properties instead of being silently dropped.
type datum struct {
	openaiutil.Datum
	Extra map[string]json.RawMessage `json:"-"`
}

func (d *datum) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &d.Datum); err != nil {
		return fmt.Errorf("unmarshaling well-known model fields: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshaling model fields: %w", err)
	}
	delete(raw, "id")
	delete(raw, "object")
	delete(raw, "created")
	delete(raw, "owned_by")
	d.Extra = raw

	return nil
}

type modelsResponse struct {
	openaiutil.ModelsResponse[datum]
}

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
	prefix := strings.TrimSpace(config.PathPrefix)
	if prefix == "" {
		return nil, fmt.Errorf("no node prefix configured: PATH_PREFIX is empty")
	}
	if strings.Contains(prefix, "/") {
		return nil, fmt.Errorf("invalid node prefix %q: PATH_PREFIX must not contain %q", prefix, "/")
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
	var resp modelsResponse
	if err := openaiutil.FetchModels(ctx, p.httpClient, p.config.BaseURL, p.config.APIKey, &resp); err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("fetching models from %q: %w", p.config.BaseURL, err)
	}
	models := resp.Data

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
			p.logger.Printf("WARN: skipping model %q reported by %q: path %q already claimed", model.ID, p.config.BaseURL, path)
			continue
		}
		seen[path] = struct{}{}

		properties := pluginapi.PropertyMap{}
		if model.OwnedBy != "" {
			properties[propertyKeyOwnedBy] = model.OwnedBy
		}
		for key, value := range model.Extra {
			setProperty(properties, key, value)
		}

		nodes = append(nodes, pluginapi.NodeClaim{
			ID:         pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: path},
			Properties: properties,
		})
	}

	return pluginapi.CollectResponse{Nodes: nodes, Relations: []pluginapi.RelationClaim{}}, nil
}

// setProperty assigns raw to properties[key], flattening JSON objects into
// dotted-path sub-keys (e.g. "meta.n_params") so their individual fields are
// queryable rather than buried in one JSON-blob property. JSON arrays are
// kept as a single raw-JSON string property, since flattening them by index
// wouldn't be meaningfully queryable.
func setProperty(properties pluginapi.PropertyMap, key string, raw json.RawMessage) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		for subKey, value := range obj {
			setProperty(properties, key+"."+subKey, value)
		}
		return
	}
	properties[key] = rawJSONToString(raw)
}

// rawJSONToString renders a JSON value as a property string: a JSON string
// value is unquoted, anything else (numbers, booleans, arrays) is rendered as
// its compact JSON text.
func rawJSONToString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}
