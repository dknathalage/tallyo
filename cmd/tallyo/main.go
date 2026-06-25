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
	dataDir := flag.String("data-dir", "", "data directory for the SQLite database (default: DATA_DIR env, else ./data)")
	secureCookie := flag.Bool("secure-cookie", false, "mark the session cookie Secure (HTTPS only)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	logLevel := flag.String("log-level", app.EnvOr("LOG_LEVEL", "info"), "log level: debug|info|warn|error")
	logFormat := flag.String("log-format", app.EnvOr("LOG_FORMAT", "json"), "log format: json (production) | text (dev)")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	cfg := app.Config{
		Port:             *port,
		DataDir:          *dataDir,
		SecureCookie:     *secureCookie,
		LogLevel:         *logLevel,
		LogFormat:        *logFormat,
		FeatureSmarts:    app.EnvBool("TALLYO_FEATURE_SMARTS", true),
		FeatureInvites:   app.EnvBool("TALLYO_FEATURE_INVITES", true),
		FeatureRecurring: app.EnvBool("TALLYO_FEATURE_RECURRING", true),
	}

	if err := app.Run(cfg, version); err != nil {
		slog.Error("fatal", slog.Any("error", err))
		os.Exit(1)
	}
}
