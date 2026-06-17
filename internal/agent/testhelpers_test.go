package agent

// Shared test helpers: live-API env resolution (RUN_LIVE_AGENT-gated tests) and
// a small int formatter used to build tool inputs without strconv in hot paths.

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// liveEnv returns the Anthropic config values, preferring the process
// environment and falling back to the repo-root .env file (searched upward from
// the test's working directory). Missing keys map to "".
func liveEnv(t *testing.T) map[string]string {
	t.Helper()
	keys := []string{"ANTHROPIC_API_KEY", "ANTHROPIC_MODEL", "ANTHROPIC_EFFORT"}
	out := make(map[string]string, len(keys))
	for i := range keys { // bounded by len(keys)
		out[keys[i]] = os.Getenv(keys[i])
	}
	if out["ANTHROPIC_API_KEY"] != "" {
		return out
	}
	dotenv := loadDotenv(t)
	for i := range keys { // bounded by len(keys)
		if out[keys[i]] == "" {
			out[keys[i]] = dotenv[keys[i]]
		}
	}
	return out
}

// loadDotenv parses the nearest .env walking up from the working directory (the
// package dir under `go test`). It returns an empty map when none is found; a
// parse is best-effort (KEY=VALUE lines, # comments, optional surrounding
// quotes). It never fails the test — a missing .env just yields a skip upstream.
func loadDotenv(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	dir, err := os.Getwd()
	if err != nil {
		return out
	}
	var path string
	for i := 0; i < 6; i++ { // bounded climb to repo root
		candidate := filepath.Join(dir, ".env")
		if _, statErr := os.Stat(candidate); statErr == nil {
			path = candidate
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if path == "" {
		return out
	}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() { // bounded by file length
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		out[k] = v
	}
	return out
}

// itoa renders a positive int64 without importing strconv into the test's hot
// path; bounded by the number of decimal digits.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 { // bounded by digit count (≤19)
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
