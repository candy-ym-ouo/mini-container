package api

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"mini-container/internal/container"
	logpkg "mini-container/internal/log"
)

func imageArchive(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	w := tar.NewWriter(&buffer)
	manifest := []byte(`{"name":"sample","tag":"latest","layers":["layer0"]}`)
	entries := []struct {
		name string
		kind byte
		data []byte
	}{
		{"manifest.json", tar.TypeReg, manifest}, {"layer0/", tar.TypeDir, nil}, {"layer0/hello", tar.TypeReg, []byte("world")}}
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Typeflag: entry.kind, Mode: 0644, Size: int64(len(entry.data))}
		if entry.kind == tar.TypeDir {
			header.Mode = 0755
		}
		if err := w.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestManagementLifecycle(t *testing.T) {
	manager, err := container.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(manager, logpkg.New(io.Discard, logpkg.Error), ""))
	defer server.Close()
	var upload bytes.Buffer
	multipartWriter := multipart.NewWriter(&upload)
	part, err := multipartWriter.CreateFormFile("file", "sample.tar")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(imageArchive(t)); err != nil {
		t.Fatal(err)
	}
	if err := multipartWriter.Close(); err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest("POST", server.URL+"/api/v1/images/import", &upload)
	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 201 {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("import status %d: %s", response.StatusCode, data)
	}
	response.Body.Close()
	body, _ := json.Marshal(CreateContainerReq{Image: "sample", Cmd: []string{"/bin/sh"}, NetworkMode: "host"})
	response, err = http.Post(server.URL+"/api/v1/containers", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 201 {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("create status %d: %s", response.StatusCode, data)
	}
	var created ContainerView
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if created.Image != "sample:latest" || created.Status != container.StatusCreated {
		t.Fatalf("created container = %#v", created)
	}
	request, _ = http.NewRequest("DELETE", server.URL+"/api/v1/containers/"+created.ID, nil)
	response, err = http.DefaultClient.Do(request)
	if err != nil || response.StatusCode != 204 {
		t.Fatalf("remove container = %v, %v", response.StatusCode, err)
	}
	response.Body.Close()
	request, _ = http.NewRequest("DELETE", server.URL+"/api/v1/images/sample:latest", nil)
	response, err = http.DefaultClient.Do(request)
	if err != nil || response.StatusCode != 204 {
		t.Fatalf("remove image = %v, %v", response.StatusCode, err)
	}
	response.Body.Close()
}
