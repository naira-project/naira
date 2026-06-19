package pluginmanager

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/naira-project/naira/catalog/pluginapi"
	pluginv1 "github.com/naira-project/naira/catalog/pluginapi/proto/plugin/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// Register connects to each plugin sidecar by its configured name and gRPC
// address and returns the registered plugins keyed by plugin name.
func Register(plugins map[string]string, logger *log.Logger) (map[string]pluginapi.Plugin, func(), error) {
	registered := make(map[string]pluginapi.Plugin, len(plugins))
	var cleanups []func()

	for name, addr := range plugins {
		if name == "" || addr == "" {
			continue
		}

		client, cleanup, err := ConnectPlugin(name, addr, logger)
		if err != nil {
			for _, c := range cleanups {
				c()
			}
			return nil, nil, fmt.Errorf("connecting to plugin %q at %q: %w", name, addr, err)
		}

		registered[name] = client
		cleanups = append(cleanups, cleanup)
	}

	cleanupAll := func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}

	return registered, cleanupAll, nil
}

// ConnectPlugin establishes a gRPC connection to the specified address.
// It includes a retry mechanism that waits up to 10 seconds for the
// connection to reach the READY state before timing out.
func ConnectPlugin(name, address string, logger *log.Logger) (pluginapi.Plugin, func(), error) {
	if logger != nil {
		logger.Printf("connecting to plugin %q at %q...", name, address)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(
		address,
		// Insecure is safe for now, plugins run as sidecars within the same pod
		// and share the localhost network namespace. Upgrade to mTLS/Service Mesh
		// if plugins are moved to separate pods.
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("creating gRPC client for plugin %q: %w", name, err)
	}

	// grpc client is lazy by default, needs to be kicked to connect
	conn.Connect()

	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			break
		}

		if !conn.WaitForStateChange(ctx, state) {
			conn.Close()
			return nil, nil, fmt.Errorf("plugin %q at %q failed to become READY within timeout", name, address)
		}
	}

	if logger != nil {
		logger.Printf("successfully connected to plugin %q at %q", name, address)
	}

	grpcClient := pluginapi.NewGRPCClient(pluginv1.NewCatalogPluginServiceClient(conn))

	return grpcClient, func() { conn.Close() }, nil
}
