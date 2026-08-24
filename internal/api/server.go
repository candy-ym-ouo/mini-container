package api

import (
	"fmt"
	"mini-container/internal/container"
	logpkg "mini-container/internal/log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type Server struct {
	manager *container.Manager
	logger  *logpkg.Logger
	webDir  string
	mux     *http.ServeMux
}

func NewServer(manager *container.Manager, logger *logpkg.Logger, webDir string) *Server {
	server := &Server{manager: manager, logger: logger, webDir: webDir, mux: http.NewServeMux()}
	server.routes()
	return server
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/v1/containers", s.listContainers)
	s.mux.HandleFunc("POST /api/v1/containers", s.createContainer)
	s.mux.HandleFunc("GET /api/v1/containers/{id}", s.getContainer)
	s.mux.HandleFunc("POST /api/v1/containers/{id}/start", s.startContainer)
	s.mux.HandleFunc("POST /api/v1/containers/{id}/stop", s.stopContainer)
	s.mux.HandleFunc("DELETE /api/v1/containers/{id}", s.removeContainer)
	s.mux.HandleFunc("POST /api/v1/containers/{id}/exec", s.execContainer)
	s.mux.HandleFunc("GET /api/v1/containers/{id}/logs", s.containerLogs)
	s.mux.HandleFunc("GET /api/v1/containers/{id}/stats", s.containerStats)
	s.mux.HandleFunc("POST /api/v1/containers/{id}/wait", s.waitContainer)
	s.mux.HandleFunc("GET /api/v1/images", s.listImages)
	s.mux.HandleFunc("POST /api/v1/images/import", s.importImage)
	s.mux.HandleFunc("GET /api/v1/images/{name}/export", s.exportImage)
	s.mux.HandleFunc("DELETE /api/v1/images/{name}", s.removeImage)
	s.mux.HandleFunc("GET /api/v1/system/info", s.systemInfo)
	s.mux.HandleFunc("GET /api/v1/system/events", s.events)
	static := http.FileServer(http.Dir(s.webDir))
	if s.webDir != "" {
		s.mux.Handle("GET /", static)
	}
}

func (s *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	started := time.Now()
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Access-Control-Allow-Origin", "*")
	writer.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
	if request.Method == http.MethodOptions {
		writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	s.mux.ServeHTTP(writer, request)
	s.logger.Info("http request", map[string]any{"method": request.Method, "path": request.URL.Path, "duration_ms": time.Since(started).Milliseconds()})
}

func (s *Server) Listen(address string) error {
	server := &http.Server{Addr: address, Handler: s, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	s.logger.Info("api listening", map[string]any{"address": address, "state": s.manager.Root()})
	return server.ListenAndServe()
}

func ResolveWebDir(value string) string {
	if value != "" {
		return value
	}
	if info, err := os.Stat("web"); err == nil && info.IsDir() {
		return "web"
	}
	executable, err := os.Executable()
	candidate := filepath.Join(filepath.Dir(executable), "web")
	if err == nil {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

func ValidateWebDir(path string) error {
	if path == "" {
		return fmt.Errorf("web directory not found; pass --web")
	}
	for _, file := range []string{"index.html", "app.js", "style.css"} {
		if _, err := os.Stat(filepath.Join(path, file)); err != nil {
			return fmt.Errorf("web asset %s: %w", file, err)
		}
	}
	return nil
}
