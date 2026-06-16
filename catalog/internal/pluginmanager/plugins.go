package pluginmanager

import (
	"fmt"
	"log"
	"net"
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

func (pc *pluginClient) Name() string {
	return pc.name
}

func Register(pluginAddresses map[string]string, logger *log.Logger) ([]pluginapi.Plugin, func(), error) {
	var registered []pluginapi.Plugin
	var cleanups []func()

	for name, addr := range pluginAddresses {
		if name == "" || addr == "" {
			continue
		}

		plugin, cleanup, err := ConnectPlugin(name, addr, logger)
		if err != nil {
			for _, c := range cleanups {
				c()
			}
			return nil, nil, fmt.Errorf("connecting to plugin %q at %q: %w", name, addr, err)
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

func ConnectPlugin(name, address string, logger *log.Logger) (pluginapi.Plugin, func(), error) {
	// TCP dial check with retries to handle sidecar startup latency
	var dialErr error
	for i := range 5 {
		var conn net.Conn
		conn, dialErr = net.DialTimeout("tcp", address, 2*time.Second)
		if dialErr == nil {
			conn.Close()
			break
		}
		if logger != nil {
			logger.Printf("waiting for plugin %q at %q to listen... (%d/5)", name, address, i+1)
		}
		time.Sleep(1 * time.Second)
	}
	if dialErr != nil {
		return nil, nil, fmt.Errorf("plugin %q at %q is not listening: %w", name, address, dialErr)
	}

	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("creating gRPC client: %w", err)
	}

	if logger != nil {
		logger.Printf("successfully connected to plugin %q at %q", name, address)
	}

	client := proto.NewCatalogPluginClient(conn)
	pc := &pluginClient{
		GRPCClient: pluginapi.NewGRPCClient(client),
		name:       name,
	}

	cleanup := func() {
		conn.Close()
	}

	return pc, cleanup, nil
}
