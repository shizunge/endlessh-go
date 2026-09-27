package app

import (
	"context"
	"flag"
	"fmt"
	"slices"
	"strconv"
	"time"

	_ "github.com/golang/glog"
	altsrc "github.com/urfave/cli-altsrc/v3"
	"github.com/urfave/cli/v3"
)

const (
	FlagConfigPath = "config_path"
	FlagHostAddr   = "host"
	FlagHostPort   = "port"
	FlagConnType   = "conn_type"
	FlagMsgDelay   = "interval"
	FlagLineLength = "line_length"
	FlagMaxClients = "max_clients"

	groupProxy       = "PROXY protocol"
	FlagProxyEnable  = "proxy_protocol_enabled"
	FlagProxyTimeout = "proxy_protocol_read_header_timeout"

	groupPrometheus          = "Prometheus metrics"
	FlagPrometheusEnable     = "prometheus_enabled"
	FlagPrometheusHost       = "prometheus_host"
	FlagPrometheusPort       = "prometheus_port"
	FlagPrometheusEntry      = "prometheus_entry"
	FlagPrometheusCleanDelay = "prometheus_clean_unseen"

	groupGeoIP        = "GeoIP"
	FlagGeoIPSupplier = "geoip_supplier"
	FlagGeoIPMaxMind  = "max_mind_db"

	groupHealthCheck       = "Healthcheck"
	FlagHealthCheckEnable  = "healthcheck_enabled"
	FlagHealthCheckHost    = "healthcheck_host"
	FlagHealthCheckPort    = "healthcheck_port"
	FlagHealthCheckOneShot = "healthcheck"

	groupLogging = "Logging"

	// Deprecated
	FlagMsgDelayLegacy             = "interval_ms"
	FlagPrometheusEnableLegacy     = "enable_prometheus"
	FlagPrometheusCleanDelayLegacy = "prometheus_clean_unseen_seconds"
	FlagProxyTimeoutLegacy         = "proxy_protocol_read_header_timeout_ms"
)

func flags(configFile *string, configSrc altsrc.Sourcer) []cli.Flag {
	return append([]cli.Flag{
		// Configuration file. This flag must stay first: when the path comes
		// from the environment, its PostParse has to run before the other
		// flags read the path through configSrc.
		&cli.StringFlag{
			Name:        FlagConfigPath,
			Usage:       "Path to a TOML configuration file. Values in the file are overridden by environment variables and command-line flags.",
			Value:       *configFile,
			Destination: configFile,
			Sources:     envVars("CONFIG_PATH"),
		},
		// Core flags
		&cli.StringFlag{
			Name:    FlagHostAddr,
			Usage:   "SSH listening address",
			Value:   "0.0.0.0",
			Sources: sources("HOST_ADDRESS", "general.host", configSrc),
		},
		&cli.Uint16SliceFlag{
			Name:    FlagHostPort,
			Usage:   "SSH listening port. You may provide multiple -port flags to listen to multiple ports.",
			Value:   []uint16{2222},
			Sources: sources("LISTEN_PORT", "general.port", configSrc),
		},
		&cli.StringFlag{
			Name:    FlagConnType,
			Usage:   "Connection type. Possible values are tcp, tcp4, tcp6",
			Value:   "tcp",
			Action:  validateConnType,
			Sources: sources("CONNECTION_TYPE", "general.conn_type", configSrc),
		},
		&cli.Int64Flag{
			Name:    FlagLineLength,
			Usage:   "Maximum banner line length",
			Value:   32,
			Sources: sources("LINE_LENGTH", "general.line_length", configSrc),
		},
		&cli.Int64Flag{
			Name:    FlagMaxClients,
			Usage:   "Maximum number of clients",
			Value:   4096,
			Sources: sources("MAX_CLIENTS", "general.max_clients", configSrc),
		},
		// PROXY protocol flags
		&cli.BoolFlag{
			Name:     FlagProxyEnable,
			Category: groupProxy,
			Usage:    "Enable PROXY protocol support. This causes the server to expect PROXY protocol headers on incoming connections.",
			Value:    false,
			Sources:  sources("PROXY_PROTOCOL_ENABLED", "proxy.protocol_enabled", configSrc),
		},
		// Prometheus metrics flags
		&cli.StringFlag{
			Name:     FlagPrometheusHost,
			Category: groupPrometheus,
			Usage:    "The address for prometheus",
			Value:    "0.0.0.0",
			Sources:  sources("PROMETHEUS_HOST", "prometheus.host", configSrc),
		},
		&cli.Uint16Flag{
			Name:     FlagPrometheusPort,
			Category: groupPrometheus,
			Usage:    "The port for prometheus",
			Value:    2112,
			Sources:  sources("PROMETHEUS_PORT", "prometheus.port", configSrc),
		},
		&cli.StringFlag{
			Name:     FlagPrometheusEntry,
			Category: groupPrometheus,
			Usage:    "Entry point for prometheus",
			Value:    "metrics",
			Sources:  sources("PROMETHEUS_ENTRY", "prometheus.entry", configSrc),
		},
		// GeoIP flags
		&cli.StringFlag{
			Name:     FlagGeoIPSupplier,
			Category: groupGeoIP,
			Usage:    `Supplier to obtain Geohash of IPs. Possible values are "off", "ip-api", "max-mind-db"`,
			Value:    "off",
			Action:   validateGeoIPSupplier,
			Sources:  sources("GEOIP_SUPPLIER", "geoip.supplier", configSrc),
		},
		&cli.StringFlag{
			Name:     FlagGeoIPMaxMind,
			Category: groupGeoIP,
			Usage:    "Path to the MaxMind DB file.",
			Value:    "",
			Sources:  sources("MAX_MIND_DB", "geoip.max_mind_db", configSrc),
		},
		// Healthcheck flags
		&cli.BoolFlag{
			Name:     FlagHealthCheckEnable,
			Category: groupHealthCheck,
			Usage:    "Enable healthcheck",
			Value:    false,
			Sources:  sources("HEALTHCHECK_ENABLED", "healthcheck.enabled", configSrc),
		},
		&cli.StringFlag{
			Name:     FlagHealthCheckHost,
			Category: groupHealthCheck,
			Usage:    "The address for healthcheck.",
			Value:    "127.0.0.1",
			Sources:  sources("HEALTHCHECK_HOST", "healthcheck.host", configSrc),
		},
		&cli.Uint16Flag{
			Name:     FlagHealthCheckPort,
			Category: groupHealthCheck,
			Usage:    "HTTP port for healthcheck; Serves JSON with status and uptime at /health.",
			Value:    51000,
			Sources:  sources("HEALTHCHECK_PORT", "healthcheck.port", configSrc),
		},
		&cli.BoolFlag{
			Name:     FlagHealthCheckOneShot,
			Category: groupHealthCheck,
			Usage:    "Perform healthcheck and exit. GET healthcheck_host:healthcheck_port/health and exit 1 if status is not ok or timeout is exceeded.",
			Value:    false,
		},
	}, glogFlags(configSrc)...)
}

// glogValue adapts a [flag.Value] registered on the standard library's
// flag.CommandLine (where glog registers its flags in init) to a [cli.Value],
// so urfave/cli parses and documents glog's flags and forwards the values back
// to glog. Requires that flag.Parse is never called.
type glogValue struct {
	f *flag.Flag
}

func (g glogValue) Set(s string) error { return g.f.Value.Set(s) }
func (g glogValue) String() string     { return g.f.Value.String() }
func (g glogValue) Get() any           { return g.f.Value.(flag.Getter).Get() }

const (
	glogFlagVerbosity = "v"
	cliFlagVerbosity  = "verbosity"
)

// glogSourceKeys maps the glog flags that may also be set from the environment
// or the configuration file to their source names. They are spelled out rather
// than derived from glog's flag names so the terse ones read well (verbosity
// instead of v). The remaining glog flags (vmodule, log_backtrace_at and
// logbuflevel) stay command-line only.
var glogSourceKeys = map[string]struct {
	env  string
	toml string
}{
	"v":               {"LOGGING_VERBOSITY", "logging.verbosity"},
	"logtostderr":     {"LOGGING_LOG_TO_STDERR", "logging.log_to_stderr"},
	"alsologtostderr": {"LOGGING_ALSO_LOG_TO_STDERR", "logging.also_log_to_stderr"},
	"stderrthreshold": {"LOGGING_STDERR_THRESHOLD", "logging.stderr_threshold"},
	"log_dir":         {"LOGGING_LOG_DIR", "logging.log_dir"},
	"log_link":        {"LOGGING_LOG_LINK", "logging.log_link"},
}

// glogFlags returns glog's flags as urfave/cli flags, bound to the values glog
// registered on flag.CommandLine. The flags listed in glogSourceKeys can also
// be set from the environment and the configuration file: urfave/cli resolves
// those through the flag's Set method, which forwards the value to glog via
// glogValue or, for the bool flags, via the flag action.
func glogFlags(configSrc altsrc.Sourcer) []cli.Flag {
	// Placeholders for the flags that take a value, so help does not fall back
	// to the adapter's type name.
	placeholders := map[string]string{
		"v":                "LEVEL",
		"vmodule":          "PATTERN=N",
		"log_backtrace_at": "FILE:N",
		"stderrthreshold":  "SEVERITY",
		"log_dir":          "DIR",
		"log_link":         "DIR",
		"logbuflevel":      "LEVEL",
	}
	names := []string{
		"v",
		"vmodule",
		"log_backtrace_at",
		"logtostderr",
		"alsologtostderr",
		"stderrthreshold",
		"log_dir",
		"log_link",
		"logbuflevel",
	}
	flags := make([]cli.Flag, 0, len(names))
	for _, name := range names {
		f := flag.CommandLine.Lookup(name)
		if f == nil {
			continue
		}
		usage := f.Usage
		if placeholder, ok := placeholders[name]; ok {
			usage = fmt.Sprintf("%s (`%s`)", usage, placeholder)
		}
		var src cli.ValueSourceChain
		if keys, ok := glogSourceKeys[name]; ok {
			src = sources(keys.env, keys.toml, configSrc)
		}
		cliName := name
		var aliases []string
		if name == glogFlagVerbosity {
			cliName = cliFlagVerbosity
			aliases = []string{glogFlagVerbosity}
		}
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			def, _ := strconv.ParseBool(f.DefValue)
			flagName := name
			flags = append(flags, &cli.BoolFlag{
				Name:     cliName,
				Aliases:  aliases,
				Category: groupLogging,
				Usage:    usage,
				Value:    def,
				Sources:  src,
				Action: func(_ context.Context, _ *cli.Command, v bool) error {
					return flag.CommandLine.Set(flagName, strconv.FormatBool(v))
				},
			})
			continue
		}
		flags = append(flags, &cli.GenericFlag{
			Name:     cliName,
			Aliases:  aliases,
			Category: groupLogging,
			Usage:    usage,
			Value:    glogValue{f},
			Sources:  src,
		})
	}
	return flags
}

// mutuallyExclusiveFlags holds the current/deprecated flag pairs. Only one of
// the two variants of each pair may be set. The deprecated variant still works
// and prints a warning; it is kept so existing command lines do not break.
func mutuallyExclusiveFlags(configSrc altsrc.Sourcer) []cli.MutuallyExclusiveFlags {
	return []cli.MutuallyExclusiveFlags{
		{
			Flags: [][]cli.Flag{
				{
					&cli.DurationFlag{
						Name:    FlagMsgDelay,
						Usage:   "Message delay",
						Value:   1000 * time.Millisecond,
						Sources: sources("INTERVAL", "general.interval", configSrc),
					},
				},
				{
					&cli.IntFlag{
						Name:        FlagMsgDelayLegacy,
						Usage:       fmt.Sprintf("Message millisecond delay (deprecated, use --%s instead)", FlagMsgDelay),
						Value:       1000,
						HideDefault: true,
						Deprecated:  fmt.Sprintf("use --%s instead", FlagMsgDelay),
					},
				},
			},
		},
		{
			Category: groupProxy,
			Flags: [][]cli.Flag{
				{
					&cli.DurationFlag{
						Name:    FlagProxyTimeout,
						Usage:   "Timeout for reading the PROXY protocol header. If the connection does not send a valid PROXY protocol header in this time, the header is ignored.",
						Value:   200 * time.Millisecond,
						Sources: sources("PROXY_PROTOCOL_READ_HEADER_TIMEOUT", "proxy.protocol_read_header_timeout", configSrc),
					},
				},
				{
					&cli.IntFlag{
						Name:        FlagProxyTimeoutLegacy,
						Usage:       fmt.Sprintf("Timeout for reading the PROXY protocol header in milliseconds. If the connection does not send a valid PROXY protocol header in this time, the header is ignored. (deprecated, use --%s instead)", FlagProxyTimeout),
						Value:       200,
						HideDefault: true,
						Deprecated:  fmt.Sprintf("use --%s instead", FlagProxyTimeout),
					},
				},
			},
		},
		{
			Category: groupPrometheus,
			Flags: [][]cli.Flag{
				{
					&cli.BoolFlag{
						Name:    FlagPrometheusEnable,
						Usage:   "Enable prometheus",
						Value:   false,
						Sources: sources("PROMETHEUS_ENABLED", "prometheus.enabled", configSrc),
					},
				},
				{
					&cli.BoolFlag{
						Name:        FlagPrometheusEnableLegacy,
						Usage:       fmt.Sprintf("Enable prometheus (deprecated, use --%s instead)", FlagPrometheusEnable),
						Value:       false,
						HideDefault: true,
						Deprecated:  fmt.Sprintf("use --%s instead", FlagPrometheusEnable),
					},
				},
			},
		},
		{
			Category: groupPrometheus,
			Flags: [][]cli.Flag{
				{
					&cli.DurationFlag{
						Name:    FlagPrometheusCleanDelay,
						Usage:   "Remove series if the IP is not seen for the given time. Set to 0s to disable.",
						Value:   0,
						Sources: sources("PROMETHEUS_CLEAN_UNSEEN", "prometheus.clean_unseen", configSrc),
					},
				},
				{
					&cli.IntFlag{
						Name:        FlagPrometheusCleanDelayLegacy,
						Usage:       fmt.Sprintf("Remove series if the IP is not seen for the given time in seconds. Set to 0 to disable. (deprecated, use --%s instead)", FlagPrometheusCleanDelay),
						Value:       0,
						HideDefault: true,
						Deprecated:  fmt.Sprintf("use --%s instead", FlagPrometheusCleanDelay),
					},
				},
			},
		},
	}
}

// currentOrDeprecated returns current, unless the deprecated flag was set, in
// which case deprecatedValue is used. The pair is mutually exclusive, so at
// most one is set; current is already the default when neither is given.
func CurrentOrDeprecated[T any](
	cmd *cli.Command,
	current T,
	deprecated string,
	getDeprecated func(*cli.Command, string) T,
) T {
	if cmd.IsSet(deprecated) {
		return getDeprecated(cmd, deprecated)
	}
	return current
}

func envVars(v string) cli.ValueSourceChain {
	return cli.EnvVars(fmt.Sprintf("ENDLESSH_%s", v))
}

func validateConnType(ctx context.Context, cmd *cli.Command, v string) error {
	var accepted = []string{"tcp", "tcp4", "tcp6"}
	if !slices.Contains(accepted, v) {
		return cli.Exit(fmt.Sprintf("Flag `%s` accepts 'tcp', 'tcp4' or 'tcp6'. Received: %s", FlagConnType, v), 2)
	}
	return nil
}

func validateGeoIPSupplier(ctx context.Context, cmd *cli.Command, v string) error {
	var accepted = []string{"off", "ip-api", "max-mind-db"}
	if !slices.Contains(accepted, v) {
		return cli.Exit(fmt.Sprintf("Flag `%s` accepts 'off', 'ip-api' or 'max-mind-db'. Received: %s", FlagGeoIPSupplier, v), 2)
	}
	return nil
}
