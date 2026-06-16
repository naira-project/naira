package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/naira-project/naira/catalog/pluginapi"
	pluginv1 "github.com/naira-project/naira/catalog/pluginapi/proto/plugin/v1"
	"go-simpler.org/env"
	"google.golang.org/grpc"
)

type pluginConfig struct {
	Enabled     bool          `env:"LITELLM_ENABLED" default:"true"`
	BaseURL     string        `env:"LITELLM_BASE_URL" default:"http://127.0.0.1:4000"`
	APIKey      string        `env:"LITELLM_API_KEY"`
	HTTPTimeout time.Duration `env:"HTTP_TIMEOUT" default:"5s"`
	Port        int           `env:"PORT" default:"50051"`
}

func main() {
	var raw pluginConfig
	if err := env.Load(&raw, nil); err != nil {
		log.Fatalf("failed to load litellm config: %v", err)
	}

	logger := log.New(os.Stdout, "litellm-plugin ", log.LstdFlags)

	httpClient := &http.Client{
		Timeout: raw.HTTPTimeout,
	}

	impl := New(httpClient, logger, Config{
		Enabled: raw.Enabled,
		BaseURL: strings.TrimSpace(raw.BaseURL),
		APIKey:  strings.TrimSpace(raw.APIKey),
	})

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", raw.Port))
	if err != nil {
		logger.Fatalf("failed to listen on port %d: %v", raw.Port, err)
	}

	s := grpc.NewServer()
	pluginv1.RegisterCatalogPluginServer(s, &pluginapi.GRPCServer{Impl: impl})

	logger.Printf("litellm plugin listening on %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		logger.Fatalf("failed to serve gRPC: %v", err)
	}
}
