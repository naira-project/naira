package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"go-simpler.org/env"

	"github.com/naira-project/naira/catalog/internal/plugins"
	"github.com/naira-project/naira/catalog/pluginapi"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <plugin-name>\n", os.Args[0])
		os.Exit(1)
	}
	pluginName := os.Args[1]
	// FIXME: make sure the plugin chosen by user is enabled

	var cfg plugins.Config
	if err := env.Load(&cfg, nil); err != nil {
		fmt.Fprintf(os.Stderr, "error: loading config: %v\n", err)
		os.Exit(1)
	}

	logger := log.New(os.Stderr, "", 0)
	logger.Printf("Loaded config: %#v\n", cfg)
	registered := plugins.Register(cfg, &http.Client{}, logger)

	var plugin pluginapi.Plugin
	for _, r := range registered {
		if r.Name() == pluginName {
			plugin = r
			break
		}
	}
	if plugin == nil {
		names := make([]string, len(registered))
		for i, p := range registered {
			names[i] = p.Name()
		}
		if len(names) == 0 {
			fmt.Fprintf(os.Stderr, "plugin %q not found (no plugins enabled)\n", pluginName)
		} else {
			fmt.Fprintf(os.Stderr, "plugin %q not found; enabled plugins: %s\n", pluginName, strings.Join(names, ", "))
		}
		os.Exit(1)
	}

	result, err := plugin.Collect(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "collect: %v\n", err)
		os.Exit(1)
	}

	printMermaid(result)
}

func printMermaid(req pluginapi.IngestionRequest) {
	fmt.Println("graph TD")

	ids := make(map[pluginapi.NodeID]string, len(req.Nodes))
	for i, node := range req.Nodes {
		mermaidID := fmt.Sprintf("n%d", i)
		ids[node.ID] = mermaidID
		label := node.ID.Kind + ": " + node.ID.Path
		fmt.Printf("    %s[\"%s\"]\n", mermaidID, label)
	}

	for _, rel := range req.Relations {
		from, fromOK := ids[rel.From]
		to, toOK := ids[rel.To]
		if !fromOK || !toOK {
			fmt.Fprintf(os.Stderr, "WARN: relation's node(s) not predeclared: %v\n", rel)
			continue
		}
		fmt.Printf("    %s -->|\"%s\"| %s\n", from, rel.Kind, to)
	}
}
