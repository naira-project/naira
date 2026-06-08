package plugins

import (
	"strings"
)

type Config struct {
	PluginsDir string
}

func LoadConfig(pluginsDir string) Config {
	return Config{
		PluginsDir: strings.TrimSpace(pluginsDir),
	}
}
