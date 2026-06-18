// Plugintest connects to a running plugin over gRPC, calls Collect, and prints
// the result as a Mermaid graph to stdout.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/naira-project/naira/catalog/pluginapi"
	pluginv1 "github.com/naira-project/naira/catalog/pluginapi/proto/plugin/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	logger := log.New(os.Stderr, "", 0)

	var addrFlag string
	flag.StringVar(&addrFlag, "addr", "50051", "gRPC server address (e.g., 'localhost:50051' or just port '50051')")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	addr := normalizeAddress(addrFlag)

	// Connect to gRPC server
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		logger.Fatalf("Error: creating gRPC client to %q: %v", addr, err)
	}
	defer conn.Close()

	grpcClient := pluginapi.NewGRPCClient(pluginv1.NewCatalogPluginServiceClient(conn))

	resp, err := grpcClient.Collect(context.Background())
	if err != nil {
		logger.Fatalf("Error: Collect call to %q failed: %v", addr, err)
	}

	printMermaid(resp, logger)
}

func normalizeAddress(addr string) string {
	if !strings.Contains(addr, ":") {
		return "localhost:" + addr
	}
	return addr
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
