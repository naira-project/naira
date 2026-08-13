package pluginmain

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func printNeo4j(req pluginapi.CollectResponse, prefix string, logger *log.Logger) {
	declared := make(map[pluginapi.NodeID]bool, len(req.Nodes))

	for _, node := range req.Nodes {
		declared[node.ID] = true
		setClause := propsToNeo4jSetClause(prefix, "n", node.Properties, func(_ int, value string) string {
			return neo4jCypherString(value)
		})
		fmt.Printf("MERGE (n:`%s` {path: %s})%s;\n",
			node.ID.Kind,
			neo4jCypherString(node.ID.Path),
			setClause)
	}

	fmt.Println()

	for _, rel := range req.Relations {
		if !declared[rel.From] || !declared[rel.To] {
			logger.Printf("WARN: relation's node(s) not predeclared: %v", rel)
			continue
		}

		fmt.Printf("MATCH (a:`%s` {path: %s}), (b:`%s` {path: %s})\n",
			rel.From.Kind, neo4jCypherString(rel.From.Path),
			rel.To.Kind, neo4jCypherString(rel.To.Path))

		setClause := propsToNeo4jSetClause(prefix, "r", rel.Properties, func(_ int, value string) string {
			return neo4jCypherString(value)
		})
		fmt.Printf("MERGE (a)-[r:`%s`]->(b)%s;\n\n", rel.Kind, setClause)
	}
}

func pushNeo4j(ctx context.Context, req pluginapi.CollectResponse, prefix, url string, logger *log.Logger) error {
	username := os.Getenv("NEO4J_USERNAME")
	if username == "" {
		username = "neo4j"
	}
	password := os.Getenv("NEO4J_PASSWORD")

	driver, err := neo4j.NewDriverWithContext(url, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		return fmt.Errorf("creating neo4j driver: %w", err)
	}
	defer driver.Close(ctx)

	session := driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		for _, node := range req.Nodes {
			params := map[string]any{"path": node.ID.Path}

			setClause := propsToNeo4jSetClause(prefix, "n", node.Properties, func(i int, value string) string {
				param := fmt.Sprintf("p%d", i)
				params[param] = value
				return "$" + param
			})

			query := fmt.Sprintf("MERGE (n:`%s` {path: $path})%s",
				node.ID.Kind, setClause)

			result, err := tx.Run(ctx, query, params)
			if err != nil {
				return nil, fmt.Errorf("merging node %v: %w", node.ID, err)
			}
			if _, err := result.Consume(ctx); err != nil {
				return nil, fmt.Errorf("consuming node result %v: %w", node.ID, err)
			}
		}

		for _, rel := range req.Relations {
			params := map[string]any{
				"fromPath": rel.From.Path,
				"toPath":   rel.To.Path,
			}

			setClause := propsToNeo4jSetClause(prefix, "r", rel.Properties, func(i int, value string) string {
				param := fmt.Sprintf("p%d", i)
				params[param] = value
				return "$" + param
			})

			query := fmt.Sprintf(
				"MATCH (a:`%s` {path: $fromPath}), (b:`%s` {path: $toPath}) MERGE (a)-[r:`%s`]->(b)%s",
				rel.From.Kind, rel.To.Kind, rel.Kind, setClause)

			result, err := tx.Run(ctx, query, params)
			if err != nil {
				return nil, fmt.Errorf("merging relation %v->%v: %w", rel.From, rel.To, err)
			}
			if _, err := result.Consume(ctx); err != nil {
				return nil, fmt.Errorf("consuming relation result %v->%v: %w", rel.From, rel.To, err)
			}
		}

		return nil, nil
	})
	if err != nil {
		return fmt.Errorf("executing write transaction: %w", err)
	}
	return nil
}

// propsToNeo4jSetClause builds a " SET varName.`prefix.key` = <value>, ..." clause, or "" if props is empty.
// valueFn renders the right-hand <value> side of each assignment (e.g. a literal or a query parameter).
func propsToNeo4jSetClause(prefix, varName string, props pluginapi.PropertyMap, valueFn func(i int, value string) string) string {
	keys := sortedKeys(props)
	if len(keys) == 0 {
		return ""
	}
	parts := make([]string, 0, len(keys))
	for i, k := range keys {
		parts = append(parts, fmt.Sprintf("%s.`%s.%s` = %s", varName, prefix, k, valueFn(i, props[k])))
	}
	return " SET " + strings.Join(parts, ", ")
}

// neo4jCypherString escapes s for use as a Cypher double-quoted string literal.
func neo4jCypherString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func sortedKeys(m pluginapi.PropertyMap) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
