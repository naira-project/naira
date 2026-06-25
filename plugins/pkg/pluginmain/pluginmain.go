package pluginmain

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"

	pluginapi "github.com/naira-project/naira/plugins/pkg/api"
	pluginv1 "github.com/naira-project/naira/plugins/pkg/api/proto/plugin/v1"
	"go-simpler.org/env"
	"google.golang.org/grpc"
)

type serverConfig struct {
	Port int `env:"PORT" default:"50051"`
}

func LoadConfig[C any](logger *log.Logger) (C, serverConfig) {
	var cfg C
	if err := env.Load(&cfg, nil); err != nil {
		logger.Fatalf("failed to load config: %v", err)
	}

	var srv serverConfig
	if err := env.Load(&srv, nil); err != nil {
		logger.Fatalf("failed to load server config: %v", err)
	}

	return cfg, srv
}

func Serve(p pluginapi.Plugin, serverConfig serverConfig, logger *log.Logger) {
	mermaidFlag := flag.Bool("mermaid", false, "Return the collect result in Mermaid format and terminate")
	flag.Parse()

	if *mermaidFlag {
		res, err := p.Collect(context.Background())
		if err != nil {
			logger.Fatalf("failed to collect: %v", err)
		}
		printMermaid(res, logger)
		return
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", serverConfig.Port))
	if err != nil {
		logger.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pluginv1.RegisterCatalogPluginServiceServer(s, &pluginapi.GRPCServer{Impl: p})

	logger.Printf("Plugin listening via gRPC on %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		logger.Fatalf("failed to serve gRPC: %v", err)
	}
}

func printMermaid(req pluginapi.CollectResponse, logger *log.Logger) {
	fmt.Println("graph TD")

	ids := make(map[pluginapi.NodeID]string, len(req.Nodes))
	for i, node := range req.Nodes {
		mermaidID := fmt.Sprintf("n%d", i)
		ids[node.ID] = mermaidID
		label := node.ID.Kind + "/" + node.ID.Path
		fmt.Printf("    %s[\"%s\"]\n", mermaidID, label)
	}

	for _, rel := range req.Relations {
		from, fromOK := ids[rel.From]
		to, toOK := ids[rel.To]
		if !fromOK || !toOK {
			logger.Printf("WARN: relation's node(s) not predeclared: %v", rel)
			continue
		}
		fmt.Printf("    %s -->|\"%s\"| %s\n", from, rel.Kind, to)
	}
}
