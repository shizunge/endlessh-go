package app

import (
	"os"
	"path/filepath"

	altsrc "github.com/urfave/cli-altsrc/v3"
	"github.com/urfave/cli/v3"
)

func Command(action cli.ActionFunc) *cli.Command {
	// The default path lives in the pointer from the start; --config_path (or
	// its environment variable) overwrites it while flags are parsed. The
	// sourcer reads it lazily, when the configuration file is looked up.
	configPath := defaultConfigPath()
	configSrc := altsrc.NewStringPtrSourcer(&configPath)
	return &cli.Command{
		Name:                   filepath.Base(os.Args[0]),
		Usage:                  "A golang implementation of endlessh (SSH tarpit) exporting Prometheus metrics, visualized by a Grafana dashboard",
		Authors:                []any{"Shizun Ge"},
		Flags:                  flags(&configPath, configSrc),
		MutuallyExclusiveFlags: mutuallyExclusiveFlags(configSrc),
		Action:                 action,
	}
}
