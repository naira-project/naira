package main

import (
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

type config struct {
	Kubeconfig string `env:"DEPL_CALLS_SVC_KUBECONFIG"`
}

func main() {
	app := pluginmain.New[config]()

	app.Serve(New(app.Config()))
}
