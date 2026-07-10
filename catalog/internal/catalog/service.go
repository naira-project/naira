package catalog

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/google/uuid"
	openfga "github.com/openfga/go-sdk"
)

var (
	ErrNodeNotFound      = errors.New("node not found")
	ErrInvalidPluginName = errors.New("invalid plugin name")
	ErrPluginNotFound    = errors.New("plugin not found")
)

// TupleWriter writes OpenFGA tuples. Satisfied by auth.OpenfgaClient without
// importing the auth package, which would create an import cycle (auth already
// imports catalog for catalog.NodeID).
type TupleWriter interface {
	WriteTuples(tuples []openfga.TupleKey) error
}

// defaultViewerRole is granted viewer access to every node synced from a plugin.
const defaultViewerRole = "role:ai-engineer#assignee"

type Service struct {
	store   Store
	plugins map[string]Plugin
	logger  *log.Logger
	fga     TupleWriter
}

func NewService(store Store, logger *log.Logger, fga TupleWriter, plugins ...Plugin) *Service {
	registeredPlugins := make(map[string]Plugin, len(plugins))
	for _, plugin := range plugins {
		if plugin == nil {
			continue
		}
		registeredPlugins[normalizePluginName(plugin.Name())] = plugin
	}

	return &Service{store: store, plugins: registeredPlugins, logger: logger, fga: fga}
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

	snapshotID := uuid.New()

	upsertedNodes, upsertedRelations, err := s.store.ApplyPluginSnapshot(pluginName, snapshotID, request.Nodes, request.Relations)
	if err != nil {
		return fmt.Errorf("upserting graph from plugin %q: %w", pluginName, err)
	}

	if s.fga != nil && len(request.Nodes) > 0 {
		tuples := make([]openfga.TupleKey, 0, len(request.Nodes))
		for _, nodeClaim := range request.Nodes {
			tuples = append(tuples, openfga.TupleKey{
				User:     defaultViewerRole,
				Relation: "viewer",
				Object:   fmt.Sprintf("naira_io_model:%s/%s", nodeClaim.ID.Kind, nodeClaim.ID.Path),
			})
		}

		if err := s.fga.WriteTuples(tuples); err != nil {
			return fmt.Errorf("granting default viewer role for plugin %q: %w", pluginName, err)
		}
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
