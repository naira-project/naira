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

	"github.com/naira-project/naira/plugins/internal/openaicompat"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

const propertyKeyOwnedBy = "owned_by"

// datum embeds openaiutil.Datum and additionally captures every field of a
// /v1/models entry that isn't one of Datum's well-known fields, so that
// provider-specific extras (e.g. llama.cpp's "meta") surface as node
// properties instead of being silently dropped.
type datum struct {
	openaicompat.Datum
	// FIXME(go1.27+): migrate to jsonv2 with `json:",embed"` and delete .UnmarshalJSON() func
	Props map[string]json.RawMessage `json:"-"`
}

func (d *datum) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &d.Datum); err != nil {
		return fmt.Errorf("unmarshaling well-known model fields: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshaling extra model fields: %w", err)
	}
	delete(raw, "id")
	d.Props = raw

	return nil
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
	var resp openaicompat.DataResponse[datum]
	if err := openaicompat.GetModels(ctx, p.httpClient, p.config.BaseURL, p.config.APIKey, &resp); err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("fetching models from %q: %w", p.config.BaseURL, err)
	}
	models := resp.Data

	nodes := make([]pluginapi.NodeClaim, 0, len(models))
	seen := make(map[string]bool, len(models))
	for _, model := range models {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			p.logger.Printf("WARN: skipping model with empty id reported by %q", p.config.BaseURL)
			continue
		}

		path := p.nodePrefix + "/" + id
		if seen[path] {
			p.logger.Printf("WARN: skipping model %q reported by %q: path %q already claimed", model.ID, p.config.BaseURL, path)
			continue
		}
		seen[path] = true

		properties := pluginapi.PropertyMap{}
		for k, v := range model.Props {
			setProperty(properties, k, v)
		}
		if len(properties) == 0 {
			properties = nil
		}

		nodes = append(nodes, pluginapi.NodeClaim{
			ID:         pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: path},
			Properties: properties,
		})
	}

	return pluginapi.CollectResponse{Nodes: nodes, Relations: []pluginapi.RelationClaim{}}, nil
}

// setProperty assigns raw to properties[key], flattening JSON objects into
// dotted-path sub-keys (e.g. "meta.n_params"). JSON strings are stripped of
// quotes; other types (incl. arrays) are assigned as raw text.
func setProperty(properties pluginapi.PropertyMap, key string, raw json.RawMessage) {
	// raw is a JSON object/map? if yes, recursively flatten keys into dotted "paths"
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		for subKey, v := range obj {
			setProperty(properties, key+"."+subKey, v)
		}
		return
	}

	// raw is a JSON string? if yes, strip quotes
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		properties[key] = s
		return
	}

	// fallback - assign raw text as string (note: includes arrays)
	properties[key] = string(raw)
}
