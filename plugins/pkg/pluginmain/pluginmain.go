package pluginmain

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	pluginv1 "github.com/naira-project/naira/plugins/pkg/pluginapi/proto/plugin/v1"
	"go-simpler.org/env"
	"google.golang.org/grpc"
)

type serverConfig struct {
	Port int `env:"PORT" default:"50051"`
}

type App[C any] struct {
	PluginConfig C
	serverConfig serverConfig
	Logger       *log.Logger
}

// New initializes the runtime environment, loads environment variables,
// and sets up a default logger.
func New[C any]() *App[C] {
	logger := log.New(os.Stdout, "", log.LstdFlags)

	var cfg C
	if err := env.Load(&cfg, nil); err != nil {
		logger.Fatalf("failed to load plugin config: %v", err)
	}

	var srv serverConfig
	if err := env.Load(&srv, nil); err != nil {
		logger.Fatalf("failed to load server config: %v", err)
	}

	return &App[C]{
		PluginConfig: cfg,
		Logger:       logger,
		serverConfig: srv,
	}
}

func (a *App[C]) Serve(p pluginapi.Plugin) {
	var (
		mermaidFlag = flag.Bool("mermaid", false, "Return the collect result in Mermaid format and terminate")
		neo4jPrefix = flag.String("neo4j", "", "Return the collect result as Neo4j Cypher with the given property prefix and terminate")
		neo4jURL    = flag.String("neo4j-url", "", "Push the collect result to the given Neo4j URL instead of printing (requires -neo4j)")
	)
	flag.Parse()

	if *mermaidFlag {
		res, err := p.Collect(context.Background())
		if err != nil {
			a.Logger.Fatalf("failed to collect: %v", err)
		}
		printMermaid(res, a.Logger)
		return
	}

	if *neo4jPrefix != "" {
		res, err := p.Collect(context.Background())
		if err != nil {
			a.Logger.Fatalf("failed to collect: %v", err)
		}
		if *neo4jURL == "" {
			printNeo4j(res, *neo4jPrefix, a.Logger)
			return
		}
		if err := pushNeo4j(context.Background(), res, *neo4jPrefix, *neo4jURL, a.Logger); err != nil {
			a.Logger.Fatalf("failed to push to neo4j: %v", err)
		}
		return
	}

	// Using 127.0.0.1 limits traffic to the local pod. Disallows cross-pod communication
	lis, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", a.serverConfig.Port))
	if err != nil {
		a.Logger.Fatalf("failed to listen on port %d: %v", a.serverConfig.Port, err)
	}

	s := grpc.NewServer()
	pluginv1.RegisterCatalogPluginServiceServer(s, &pluginapi.GRPCServer{Impl: p})

	a.Logger.Printf("Plugin listening via gRPC on %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		a.Logger.Fatalf("failed to serve gRPC: %v", err)
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
