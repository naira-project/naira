package main

import (
	"log"
	"os"

	"github.com/naira-project/naira/plugins/internal/pluginmain"
	"go-simpler.org/env"
)

type pluginConfig struct {
	Kubeconfig string `env:"DEPL_CALLS_SVC_KUBECONFIG"`
	Port       int    `env:"DEPL_CALLS_SVC_PORT" default:"50053"`
}

func main() {
	var raw pluginConfig
	if err := env.Load(&raw, nil); err != nil {
		log.Fatalf("failed to load depl_calls_svc config: %v", err)
	}

	logger := log.New(os.Stdout, "", log.LstdFlags)

	impl := New(config{
		kubeconfig: raw.Kubeconfig,
	})

	pluginmain.Run(impl, raw.Port, logger)
}
