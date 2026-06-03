package catalog

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrNodeNotFound      = errors.New("node not found")
	ErrInvalidPluginName = errors.New("invalid plugin name")
	ErrPluginNotFound    = errors.New("plugin not found")
)

type Service struct {
	store   Store
	plugins map[string]Plugin
	logger  *log.Logger
}

func NewService(store Store, logger *log.Logger, plugins ...Plugin) *Service {
	registeredPlugins := make(map[string]Plugin, len(plugins))
	for _, plugin := range plugins {
		if plugin == nil {
			continue
		}
		registeredPlugins[normalizePluginName(plugin.Name())] = plugin
	}

	return &Service{store: store, plugins: registeredPlugins, logger: logger}
}

func (s *Service) RunPlugin(ctx context.Context, pluginName string) error {
	pluginName = normalizePluginName(pluginName)
	if pluginName == "" {
		return fmt.Errorf("normalize plugin name: %w", ErrInvalidPluginName)
	}

	plugin, ok := s.plugins[pluginName]
	if !ok {
		return fmt.Errorf("looking up plugin %q: %w", pluginName, ErrPluginNotFound)
	}

	request, err := plugin.Collect(ctx)
	if err != nil {
		return fmt.Errorf("collecting response from plugin %q: %w", pluginName, err)
	}

	snapshotID, err := uuid.NewUUID()
	if err != nil {
		return fmt.Errorf("generating snapshot ID: %w", err)
	}

	upsertedNodes, upsertedRelations, err := s.store.UpsertGraph(pluginName, snapshotID, request.Nodes, request.Relations)
	if err != nil {
		return fmt.Errorf("upserting graph from plugin %q: %w", pluginName, err)
	}

	if s.logger != nil {
		s.logger.Printf("plugin %q upserted %d nodes and %d relations", plugin.Name(), upsertedNodes, upsertedRelations)
	}

	return nil
}

func (s *Service) RunAllPlugins(ctx context.Context) RunPluginsResult {
	pluginNames := make([]string, 0, len(s.plugins))
	for name := range s.plugins {
		pluginNames = append(pluginNames, name)
	}
	sort.Strings(pluginNames)

	response := RunPluginsResult{
		Results: make([]RunPluginResult, 0, len(pluginNames)),
	}

	for _, pluginName := range pluginNames {
		err := s.RunPlugin(ctx, pluginName)
		if err != nil {
			response.Results = append(response.Results, RunPluginResult{Plugin: pluginName, Error: err.Error()})
			continue
		}
		response.Results = append(response.Results, RunPluginResult{Plugin: pluginName})
	}

	return response
}

func (s *Service) GetNode(_ context.Context, id NodeID) (Node, error) {
	return s.store.GetNode(id)
}

func (s *Service) ListNodes(_ context.Context) []Node {
	return s.store.ListNodes()
}

func (s *Service) ListRelations(_ context.Context) []Relation {
	return s.store.ListRelations()
}

func normalizePluginName(pluginName string) string {
	return strings.TrimSpace(strings.ToLower(pluginName))
}
