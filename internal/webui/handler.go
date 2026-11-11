package webui

import (
	_ "embed"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/config"
)

// welcomeHTML 是 "/" 落地页的 Go fallback（webui 未构建或直接访问根路径时展示）。
// 其内联 i18n 字典（key: badge/subtitle/admin/apiStatus/f1t~f4t/f1d~f4d/footer/
// themeLabel/langLabel）与 webui/src/locales/{zh,en}.json 的 landing.* 键对应——
// React 落地页（webui/src/components/LandingPage.jsx）使用同一落地页的 webui 版本。
// 维护时请同步更新两边，避免两边文案漂移。
//
//go:embed welcome.html
var welcomeHTML string

type Handler struct {
	StaticDir string
}

func NewHandler() *Handler {
	return &Handler{StaticDir: resolveStaticAdminDir(config.StaticAdminDir())}
}

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/", h.index)
	r.Get("/admin", h.admin)
}

func (h *Handler) HandleAdminFallback(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if !strings.HasPrefix(r.URL.Path, "/admin/") {
		return false
	}
	h.admin(w, r)
	return true
}

// HandleAdminNavigationFallback serves the SPA index.html only for requests that
// look like a browser navigation (full page load): GET, no Authorization header,
// and Accept: text/html. It exists for SPA routes that collide with an admin API
// endpoint registered under a different method only (e.g. POST /admin/test,
// POST /admin/import): chi answers those with 405 before any handler or auth
// middleware runs, so neither the 404 nor the 401 SPA fallback can kick in and a
// hard refresh of /admin/test used to fail with HTTP 405. API clients (which
// send Authorization and/or a non-HTML Accept) still receive the 405.
func (h *Handler) HandleAdminNavigationFallback(w http.ResponseWriter, r *http.Request) bool {
	if !isBrowserNavigation(r) {
		return false
	}
	return h.HandleAdminFallback(w, r)
}

func isBrowserNavigation(r *http.Request) bool {
	if r == nil || r.Method != http.MethodGet {
		return false
	}
	if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
		return false
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/html")
}

func (h *Handler) index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(welcomeHTML))
}

func (h *Handler) admin(w http.ResponseWriter, r *http.Request) {
	staticDir := resolveStaticAdminDir(h.StaticDir)
	if fi, err := os.Stat(staticDir); err == nil && fi.IsDir() {
		h.serveFromDisk(w, r, staticDir)
		return
	}
	http.Error(w, "WebUI not built. Run `cd webui && npm run build` first.", http.StatusNotFound)
}

// staticContentTypes pins the Content-Type of common WebUI assets so we do not
// rely on mime.TypeByExtension, which on Windows consults the registry and can
// return the wrong type (e.g. application/xml for .css) when third-party
// software has overwritten HKEY_CLASSES_ROOT entries. Browsers strictly enforce
// stylesheet/script MIME types and will refuse to apply a misidentified asset,
// breaking the /admin page on affected machines.
var staticContentTypes = map[string]string{
	".css":   "text/css; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".mjs":   "text/javascript; charset=utf-8",
	".html":  "text/html; charset=utf-8",
	".htm":   "text/html; charset=utf-8",
	".json":  "application/json; charset=utf-8",
	".map":   "application/json; charset=utf-8",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".webp":  "image/webp",
	".ico":   "image/x-icon",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":   "font/ttf",
	".otf":   "font/otf",
	".txt":   "text/plain; charset=utf-8",
	".wasm":  "application/wasm",
}

// setStaticContentType pins the response Content-Type by file extension so that
// http.ServeFile does not fall back to mime.TypeByExtension (which on Windows
// reads the registry and may return an incorrect type).
func setStaticContentType(w http.ResponseWriter, fullPath string) {
	ext := strings.ToLower(filepath.Ext(fullPath))
	if ct, ok := staticContentTypes[ext]; ok {
		w.Header().Set("Content-Type", ct)
	}
}

func (h *Handler) serveFromDisk(w http.ResponseWriter, r *http.Request, staticDir string) {
	root := filepath.Clean(staticDir)
	path := strings.TrimPrefix(r.URL.Path, "/admin")
	path = strings.TrimPrefix(path, "/")
	if path != "" && strings.Contains(path, ".") {
		full := filepath.Join(root, filepath.Clean(path))
		if !isPathInsideRoot(full, root) {
			http.NotFound(w, r)
			return
		}
		if _, err := os.Stat(full); err == nil {
			if strings.HasPrefix(path, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-store, must-revalidate")
			}
			serveAdminFile(w, r, full)
			return
		}
		http.NotFound(w, r)
		return
	}
	index := filepath.Join(root, "index.html")
	if _, err := os.Stat(index); err != nil {
		http.Error(w, "WebUI index not found. Run `cd webui && npm run build` first.", http.StatusNotFound)
		return
	}
	w.Header().Set("Cache-Control", "no-store, must-revalidate")
	serveAdminFile(w, r, index)
}

func isPathInsideRoot(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func resolveStaticAdminDir(configured string) string {
	// 显式配置优先：路径可能尚未存在（auto-build 会创建），无条件信任。
	if strings.TrimSpace(os.Getenv("DS2API_STATIC_ADMIN_DIR")) != "" {
		return configured
	}
	candidates := []string{
		configured,
		filepath.Join("..", "static", "admin"),
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "static", "admin"),
			filepath.Join(exeDir, "..", "static", "admin"),
		)
	}
	if config.IsVercel() {
		candidates = append(candidates, "/var/task/static/admin")
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			return c
		}
	}
	return configured
}
