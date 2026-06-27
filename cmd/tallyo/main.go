package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/dknathalage/tallyo/internal/app"
)

// version is the build version. Source builds report "dev"; to stamp a release
// build, pass -ldflags="-X main.version=$(git describe --tags)".
var version = "dev"

func main() {
	port := flag.Int("port", 8080, "HTTP listen port")
	showVersion := flag.Bool("version", false, "print the version and exit")
	logLevel := flag.String("log-level", app.EnvOr("LOG_LEVEL", "info"), "log level: debug|info|warn|error")
	logFormat := flag.String("log-format", app.EnvOr("LOG_FORMAT", "json"), "log format: json (production) | text (dev)")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	cfg := app.Config{
		Port:           *port,
		LogLevel:       *logLevel,
		LogFormat:      *logFormat,
		FeatureSmarts:  app.EnvBool("TALLYO_FEATURE_SMARTS", true),
		FeatureInvites: app.EnvBool("TALLYO_FEATURE_INVITES", true),
	}

	if err := app.Run(cfg, version); err != nil {
		slog.Error("fatal", slog.Any("error", err))
		os.Exit(1)
	}
}
