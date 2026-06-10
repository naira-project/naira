// Plugintest runs a selected plugin standalone, printing its results to stdout
// in Mermaid graph format.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"

	"go-simpler.org/env"

	"github.com/naira-project/naira/catalog/internal/plugins"
	"github.com/naira-project/naira/catalog/pluginapi"
)

func main() {
	selectedName := ""
	if len(os.Args) > 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <plugin-name>\n", os.Args[0])
		os.Exit(1)
	}
	if len(os.Args) == 2 {
		selectedName = os.Args[1]
	}
	// if no args provided, we'll want to still print the list of available plugins

	var cfg plugins.Config
	if err := env.Load(&cfg, nil); err != nil {
		fmt.Fprintf(os.Stderr, "error: loading config: %v\n", err)
		os.Exit(1)
	}
	forceEnablePlugins(&cfg)
	registered := plugins.Register(cfg, &http.Client{}, log.New(os.Stderr, "", 0))

	pluginsByName := make(map[string]pluginapi.Plugin, len(registered))
	pluginNames := make([]string, 0, len(registered))
	for _, p := range registered {
		pluginsByName[p.Name()] = p
		pluginNames = append(pluginNames, p.Name())
	}

	if selectedName == "" {
		fmt.Fprintf(os.Stderr, "usage: %s <plugin-name>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "no plugin specified; enabled plugins: %s\n", strings.Join(pluginNames, ", "))
		os.Exit(1)
	}
	plugin := pluginsByName[selectedName]
	if plugin == nil {
		fmt.Fprintf(os.Stderr, "error: plugin %q not found; enabled plugins: %s\n", selectedName, strings.Join(pluginNames, ", "))
		os.Exit(1)
	}

	result, err := plugin.Collect(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error running plugin: %v\n", err)
		os.Exit(1)
	}

	printMermaid(result)
}

// forceEnablePlugins sets Enabled=true on every sub-struct in cfg that has a
// bool field named "Enabled".
func forceEnablePlugins(cfg *plugins.Config) {
	v := reflect.ValueOf(cfg).Elem()
	for _, f := range v.Fields() {
		if f.Kind() != reflect.Struct {
			continue
		}
		enabled := f.FieldByName("Enabled")
		if enabled.IsValid() && enabled.Kind() == reflect.Bool {
			enabled.SetBool(true)
		}
	}
}

func printMermaid(req pluginapi.IngestionRequest) {
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
			fmt.Fprintf(os.Stderr, "WARN: relation's node(s) not predeclared: %v\n", rel)
			continue
		}
		fmt.Printf("    %s -->|\"%s\"| %s\n", from, rel.Kind, to)
	}
}