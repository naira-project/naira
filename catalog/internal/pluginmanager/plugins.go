package pluginmanager

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	pluginv1 "github.com/naira-project/naira/plugins/pkg/pluginapi/proto/plugin/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// Register connects to each plugin sidecar by its configured name and gRPC
// address and returns the registered plugins keyed by plugin name.
func Register(plugins map[string]string, logger *log.Logger, timeout time.Duration) (map[string]pluginapi.Plugin, func(), error) {
	registered := make(map[string]pluginapi.Plugin, len(plugins))
	var cleanups []func()

	for name, addr := range plugins {
		if name == "" || addr == "" {
			continue
		}

		client, cleanup, err := ConnectPlugin(name, addr, logger, timeout)
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
// It includes a retry mechanism that waits up to the given timeout for the
// connection to reach the READY state before timing out.
func ConnectPlugin(name, address string, logger *log.Logger, timeout time.Duration) (pluginapi.Plugin, func(), error) {
	if logger != nil {
		logger.Printf("connecting to plugin %q at %q...", name, address)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.NewClient(
		address,
		// Insecure is acceptable here because plugins run as sidecars within the
		// same pod and share its network namespace. Traffic over localhost never
		// crosses the pod boundary and cannot be intercepted by other pods or nodes.
		// See: https://kubernetes.io/docs/concepts/workloads/pods/#pod-networking
		//
		// TODO(security): Add mTLS for defense in depth — even on localhost an
		// attacker who has compromised the pod could intercept plaintext traffic.
		// This also becomes required if plugins are ever moved to separate pods.
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
