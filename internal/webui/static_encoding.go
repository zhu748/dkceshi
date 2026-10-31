package webui

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func serveAdminFile(w http.ResponseWriter, r *http.Request, full string) {
	setStaticContentType(w, full)
	switch strings.ToLower(filepath.Ext(full)) {
	case ".html", ".js", ".mjs", ".css", ".svg", ".json":
		w.Header().Add("Vary", "Accept-Encoding")
		// Ranges refer to the identity representation, not compressed offsets.
		if r.Header.Get("Range") == "" && acceptsGzip(strings.Join(r.Header.Values("Accept-Encoding"), ",")) {
			original, originalErr := os.Stat(full)
			compressed, compressedErr := os.Stat(full + ".gz")
			if originalErr == nil && compressedErr == nil && compressed.Mode().IsRegular() &&
				!compressed.ModTime().Before(original.ModTime()) {
				w.Header().Set("Content-Encoding", "gzip")
				w.Header().Set("Content-Length", strconv.FormatInt(compressed.Size(), 10))
				http.ServeFile(w, r, full+".gz")
				return
			}
		}
	}
	http.ServeFile(w, r, full)
}

func acceptsGzip(header string) bool {
	wildcard := false
	for _, coding := range strings.Split(header, ",") {
		parts := strings.Split(coding, ";")
		name := strings.TrimSpace(parts[0])
		quality := 1.0
		for _, param := range parts[1:] {
			key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
			if ok && strings.EqualFold(strings.TrimSpace(key), "q") {
				parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
				quality = parsed
				if err != nil {
					quality = 0
				}
			}
		}
		allowed := quality > 0 && quality <= 1
		if strings.EqualFold(name, "gzip") {
			return allowed
		}
		if name == "*" {
			wildcard = allowed
		}
	}
	return wildcard
}
