package app

import (
	"os"
	"path/filepath"
	"strings"

	altsrc "github.com/urfave/cli-altsrc/v3"
	"github.com/urfave/cli-altsrc/v3/toml"
	"github.com/urfave/cli/v3"
)

const defaultConfigPathTemplate = "${XDG_CONFIG_HOME:-$HOME/.config}/endlessh/config.toml"

func defaultConfigPath() string {
	path := defaultConfigPathTemplate
	// Nothing to expand. This also makes repeated expansion of an already
	// expanded path a no-op.
	if !strings.ContainsAny(path, "$~") {
		return path
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	var expand func(string) string
	expand = func(s string) string {
		return os.Expand(s, func(key string) string {
			name, def, hasDefault := strings.Cut(key, ":-")
			if value, ok := os.LookupEnv(name); ok && value != "" {
				return value
			}
			if hasDefault {
				return expand(def)
			}
			return os.Getenv(name)
		})
	}
	return expand(path)
}

func sources(envVar, tomlKey string, configSrc altsrc.Sourcer) cli.ValueSourceChain {
	chain := envVars(envVar)
	if configSrc != nil {
		chain.Chain = append(chain.Chain, toml.TOML(tomlKey, configSrc))
	}
	return chain
}
