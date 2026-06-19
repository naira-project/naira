package pluginmain

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/naira-project/naira/catalog/pluginapi"
	pluginv1 "github.com/naira-project/naira/catalog/pluginapi/proto/plugin/v1"
	"google.golang.org/grpc"
)

func Run(p pluginapi.Plugin, defaultPort int, logger *log.Logger) {
	mermaidFlag := flag.Bool("mermaid", false, "Return the collect result in Mermaid format and terminate")
	portFlag := flag.Int("port", defaultPort, "Port for the gRPC server")
	flag.Parse()

	if *mermaidFlag {
		res, err := p.Collect(context.Background())
		if err != nil {
			log.Fatalf("failed to collect: %v", err)
		}
		printMermaid(res, nil)
		return
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *portFlag))
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
