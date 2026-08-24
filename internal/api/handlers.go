package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mini-container/internal/container"
	execpkg "mini-container/internal/exec"
	"mini-container/internal/image"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, err error) {
	status, code := 500, "INTERNAL"
	switch {
	case errors.Is(err, container.ErrInvalidParam):
		status, code = 400, "INVALID_PARAM"
	case errors.Is(err, container.ErrNotFound), errors.Is(err, image.ErrNotFound):
		status, code = 404, "NOT_FOUND"
	case errors.Is(err, container.ErrImageMissing):
		status, code = 404, "IMAGE_MISSING"
	case errors.Is(err, container.ErrConflict), errors.Is(err, image.ErrReferenced):
		status, code = 409, "CONFLICT"
	case errors.Is(err, container.ErrNotSupported):
		status, code = 501, "NOT_SUPPORTED"
	}
	writeJSON(w, status, ErrorEnvelope{Error: APIError{Code: code, Message: err.Error()}})
}
func decodeJSON(r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return fmt.Errorf("%w: Content-Type must be application/json", container.ErrInvalidParam)
	}
	d := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return fmt.Errorf("%w: invalid JSON: %v", container.ErrInvalidParam, err)
	}
	if err := d.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: request must contain one JSON value", container.ErrInvalidParam)
	}
	return nil
}
func (s *Server) listContainers(w http.ResponseWriter, r *http.Request) {
	items := s.manager.List(container.Status(r.URL.Query().Get("status")))
	out := make([]ContainerView, len(items))
	for i, c := range items {
		out[i] = view(c, nil)
	}
	writeJSON(w, 200, out)
}
func (s *Server) createContainer(w http.ResponseWriter, r *http.Request) {
	var b CreateContainerReq
	if err := decodeJSON(r, &b); err != nil {
		writeError(w, err)
		return
	}
	c, err := s.manager.Create(container.CreateOptions{Name: b.Name, Image: b.Image, Spec: container.Spec{Cmd: b.Cmd, Hostname: b.Hostname, NetworkMode: b.NetworkMode, Resources: b.Resources, Mounts: b.Mounts, Env: b.Env}, AutoRemove: b.AutoRemove})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 201, view(c, nil))
}
func (s *Server) getContainer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.manager.Get(id)
	if err != nil {
		writeError(w, err)
		return
	}
	stats, e := s.manager.Stats(id)
	if e != nil {
		writeJSON(w, 200, view(c, nil))
		return
	}
	writeJSON(w, 200, view(c, &stats))
}
func (s *Server) startContainer(w http.ResponseWriter, r *http.Request) {
	c, err := s.manager.Start(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, view(c, nil))
}
func (s *Server) stopContainer(w http.ResponseWriter, r *http.Request) {
	seconds := 10
	if v := r.URL.Query().Get("timeout"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 0 || n > 300 {
			writeError(w, fmt.Errorf("%w: timeout must be 0..300", container.ErrInvalidParam))
			return
		}
		seconds = n
	}
	c, err := s.manager.Stop(r.PathValue("id"), time.Duration(seconds)*time.Second)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, view(c, nil))
}
func (s *Server) removeContainer(w http.ResponseWriter, r *http.Request) {
	if err := s.manager.Remove(r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) execContainer(w http.ResponseWriter, r *http.Request) {
	var b ExecReq
	if err := decodeJSON(r, &b); err != nil {
		writeError(w, err)
		return
	}
	if len(b.Cmd) == 0 {
		writeError(w, fmt.Errorf("%w: cmd is required", container.ErrInvalidParam))
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	flusher, _ := w.(http.Flusher)
	outR, outW := io.Pipe()
	errR, errW := io.Pipe()
	done := make(chan struct{})
	go func() {
		_ = execpkg.CopyFrames(outR, errR, w)
		if flusher != nil {
			flusher.Flush()
		}
		close(done)
	}()
	code, err := s.manager.Exec(r.PathValue("id"), b.Cmd, execpkg.Streams{Stdin: r.Body, Stdout: outW, Stderr: errW})
	outW.Close()
	errW.Close()
	<-done
	if err != nil {
		_ = json.NewEncoder(w).Encode(execpkg.Frame{Stream: "stderr", Data: err.Error() + "\n"})
		code = -1
	}
	_ = json.NewEncoder(w).Encode(execpkg.Frame{Stream: "exit", ExitCode: code})
}
func (s *Server) containerLogs(w http.ResponseWriter, r *http.Request) {
	lines, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	data, err := s.manager.Logs(r.PathValue("id"), lines)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, data)
}
func (s *Server) containerStats(w http.ResponseWriter, r *http.Request) {
	v, err := s.manager.Stats(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) waitContainer(w http.ResponseWriter, r *http.Request) {
	v, err := s.manager.Wait(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]int{"exitCode": v})
}
func (s *Server) listImages(w http.ResponseWriter, _ *http.Request) {
	v, err := s.manager.Images().List()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) importImage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<30)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, err)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, fmt.Errorf("%w: file field is required", container.ErrInvalidParam))
		return
	}
	defer file.Close()
	tmp, err := os.CreateTemp("", "mini-container-import-*.tar")
	if err != nil {
		writeError(w, err)
		return
	}
	path := tmp.Name()
	defer os.Remove(path)
	if _, err = io.Copy(tmp, file); err == nil {
		err = tmp.Close()
	}
	if err != nil {
		writeError(w, err)
		return
	}
	item, err := s.manager.Images().Import(path)
	if err != nil {
		writeError(w, fmt.Errorf("import %s: %w", header.Filename, err))
		return
	}
	writeJSON(w, 201, item)
}
func (s *Server) exportImage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tmp, err := os.CreateTemp("", "mini-container-export-*.tar")
	if err != nil {
		writeError(w, err)
		return
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)
	if err = s.manager.Images().Export(name, path); err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(strings.ReplaceAll(name, ":", "-"))+`.tar"`)
	http.ServeFile(w, r, path)
}
func (s *Server) removeImage(w http.ResponseWriter, r *http.Request) {
	if err := s.manager.Images().Remove(r.PathValue("name")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(204)
}
func kernelVersion() string {
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return runtime.GOOS
	}
	return strings.TrimSpace(string(out))
}
func (s *Server) systemInfo(w http.ResponseWriter, _ *http.Request) {
	version := 0
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		version = 2
	} else if _, err := os.Stat("/sys/fs/cgroup"); err == nil {
		version = 1
	}
	images, _ := s.manager.Images().List()
	writeJSON(w, 200, SystemInfo{CgroupVersion: version, Kernel: kernelVersion(), GoVersion: runtime.Version(), StateDir: s.manager.Root(), Containers: len(s.manager.List("")), Images: len(images), Subnet: "10.0.42.0/24"})
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch, cancel := s.manager.Subscribe()
	defer func() { cancel() }()
	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
