package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/naira-project/naira/catalog/pluginapi"
	"github.com/naira-project/naira/catalog/pluginapi/proto"
	"go-simpler.org/env"
	"google.golang.org/grpc"
)

type pluginConfig struct {
	Enabled     bool          `env:"MLFLOW_ENABLED" default:"true"`
	BaseURL     string        `env:"MLFLOW_BASE_URL" default:"http://127.0.0.1:5000"`
	BearerToken string        `env:"MLFLOW_BEARER_TOKEN"`
	HTTPTimeout time.Duration `env:"HTTP_TIMEOUT" default:"5s"`
	Port        int           `env:"PORT" default:"50052"`
}

func main() {
	var raw pluginConfig
	if err := env.Load(&raw, nil); err != nil {
		log.Fatalf("failed to load mlflow config: %v", err)
	}

	httpClient := &http.Client{
		Timeout: raw.HTTPTimeout,
	}

	impl := New(httpClient, Config{
		Enabled:     raw.Enabled,
		BaseURL:     strings.TrimSpace(raw.BaseURL),
		BearerToken: strings.TrimSpace(raw.BearerToken),
	})

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", raw.Port))
	if err != nil {
		log.Fatalf("failed to listen on port %d: %v", raw.Port, err)
	}

	s := grpc.NewServer()
	proto.RegisterCatalogPluginServer(s, &pluginapi.GRPCServer{Impl: impl})

	log.Printf("mlflow plugin listening on %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve gRPC: %v", err)
	}
}
