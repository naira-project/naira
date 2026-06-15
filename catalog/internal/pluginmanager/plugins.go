package pluginmanager

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/naira-project/naira/catalog/pluginapi"
	"github.com/naira-project/naira/catalog/pluginapi/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type pluginClient struct {
	*pluginapi.GRPCClient
	name string
}

// todo: think who should define name. configmap which catalog will read?
func (pc *pluginClient) Name() string {
	return pc.name
}

func Register(addresses []string, logger *log.Logger) ([]pluginapi.Plugin, func(), error) {
	var registered []pluginapi.Plugin
	var cleanups []func()

	for _, addr := range addresses {
		if addr == "" {
			continue
		}

		plugin, cleanup, err := ConnectPlugin(addr, logger)
		if err != nil {
			for _, c := range cleanups {
				c()
			}
			return nil, nil, fmt.Errorf("connecting to plugin at %q: %w", addr, err)
		}

		registered = append(registered, plugin)
		cleanups = append(cleanups, cleanup)
	}

	cleanupAll := func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}

	return registered, cleanupAll, nil
}

func ConnectPlugin(address string, logger *log.Logger) (pluginapi.Plugin, func(), error) {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("creating gRPC client: %w", err)
	}

	client := proto.NewCatalogPluginClient(conn)

	var resp *proto.NameResponse
	for i := range 5 {
		nameCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		resp, err = client.Name(nameCtx, &proto.Empty{})
		cancel()
		if err == nil {
			break
		}
		if logger != nil {
			logger.Printf("waiting for plugin at %q to be ready... (%d/5)", address, i+1)
		}
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("getting plugin name from %q: %w", address, err)
	}

	if logger != nil {
		logger.Printf("successfully connected to plugin %q at %q", resp.Name, address)
	}

	pc := &pluginClient{
		GRPCClient: pluginapi.NewGRPCClient(client),
		name:       resp.Name,
	}

	cleanup := func() {
		conn.Close()
	}

	return pc, cleanup, nil
}
