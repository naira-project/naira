package main

import (
	"log"
	"os"

	"github.com/naira-project/naira/plugins/internal/pluginmain"
)

type config struct {
	Kubeconfig string `env:"DEPL_CALLS_SVC_KUBECONFIG"`
}

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	cfg, srvCfg := pluginmain.LoadConfig[config](logger)

	pluginmain.Serve(New(cfg), srvCfg, logger)
}
