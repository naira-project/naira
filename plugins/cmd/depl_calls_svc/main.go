package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/naira-project/naira/catalog/pluginapi"
	pluginv1 "github.com/naira-project/naira/catalog/pluginapi/proto/plugin/v1"
	"go-simpler.org/env"
	"google.golang.org/grpc"
)

type pluginConfig struct {
	Kubeconfig string `env:"DEPL_CALLS_SVC_KUBECONFIG"`
	Port       int    `env:"DEPL_CALLS_SVC_PORT" default:"50053"`
}

func main() {
	var raw pluginConfig
	if err := env.Load(&raw, nil); err != nil {
		log.Fatalf("failed to load depl_calls_svc config: %v", err)
	}

	logger := log.New(os.Stdout, "depl_calls_svc-plugin ", log.LstdFlags)

	impl := New(config{
		kubeconfig: raw.Kubeconfig,
	})

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", raw.Port))
	if err != nil {
		logger.Fatalf("failed to listen on port %d: %v", raw.Port, err)
	}

	s := grpc.NewServer()
	pluginv1.RegisterCatalogPluginServiceServer(s, &pluginapi.GRPCServer{Impl: impl})

	logger.Printf("depl_calls_svc plugin listening on %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		logger.Fatalf("failed to serve gRPC: %v", err)
	}
}
