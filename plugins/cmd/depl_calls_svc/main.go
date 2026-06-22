package main

import (
	"log"
	"os"

	"github.com/naira-project/naira/plugins/internal/pluginmain"
	"go-simpler.org/env"
)

const defaultPort = 50053

type pluginConfig struct {
	Kubeconfig string `env:"DEPL_CALLS_SVC_KUBECONFIG"`
}

func main() {
	var raw pluginConfig
	if err := env.Load(&raw, nil); err != nil {
		log.Fatalf("failed to load depl_calls_svc config: %v", err)
	}

	logger := log.New(os.Stdout, "", log.LstdFlags)

	impl := New(raw)

	pluginmain.Run(impl, defaultPort, logger)
}
