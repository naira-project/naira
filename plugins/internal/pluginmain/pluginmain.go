package pluginmain

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"

	"github.com/naira-project/naira/catalog/pluginapi"
	pluginv1 "github.com/naira-project/naira/catalog/pluginapi/proto/plugin/v1"
	"google.golang.org/grpc"
)

const (
	portEnv     = "PORT"
	defaultPort = 50051
)

func Run(p pluginapi.Plugin, logger *log.Logger) {
	mermaidFlag := flag.Bool("mermaid", false, "Return the collect result in Mermaid format and terminate")
	flag.Parse()

	if *mermaidFlag {
		res, err := p.Collect(context.Background())
		if err != nil {
			log.Fatalf("failed to collect: %v", err)
		}
		printMermaid(res, nil)
		return
	}

	addr := fmt.Sprintf(":%d", getPort(logger))
	lis, err := net.Listen("tcp", addr)
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

func getPort(logger *log.Logger) int {
	envPort := os.Getenv(portEnv)
	if envPort == "" {
		return defaultPort
	}

	port, err := strconv.Atoi(envPort)
	if err != nil {
		logger.Fatalf("invalid PORT value %q: %v", envPort, err)
	}
	return port
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
