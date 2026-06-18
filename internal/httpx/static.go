package httpx

import (
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

// spaFallback is the prerendered shell served for any path the SPA owns.
const spaFallback = "200.html"

// SPAHandler serves static assets from fsys, falling back to 200.html (the
// SvelteKit SPA shell) for any path that does not map to a real file. The
// fallback always returns 200 so the SPA can render its own client routes and
// 404 pages. fsys must be non-nil.
func SPAHandler(fsys fs.FS) http.Handler {
	if fsys == nil {
		panic("httpx.SPAHandler: nil fs.FS")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if data, ok := readFile(fsys, name); ok {
			serveBytes(w, name, data)
			return
		}
		data, ok := readFile(fsys, spaFallback)
		if !ok {
			http.Error(w, "spa shell missing", http.StatusInternalServerError)
			return
		}
		serveBytes(w, spaFallback, data)
	})
}

// readFile returns the file contents when name names an existing regular file.
// Empty names and directories report not-found.
func readFile(fsys fs.FS, name string) ([]byte, bool) {
	if name == "" {
		return nil, false
	}
	info, err := fs.Stat(fsys, name)
	if err != nil || info.IsDir() {
		return nil, false
	}
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, false
	}
	return data, true
}

// serveBytes writes data with a Content-Type derived from the file extension,
// sniffing the body when the extension is unknown.
func serveBytes(w http.ResponseWriter, name string, data []byte) {
	ct := mime.TypeByExtension(filepath.Ext(name))
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		// Client likely disconnected; nothing actionable beyond logging.
		return
	}
}
