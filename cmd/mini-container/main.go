package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"mini-container/internal/api"
	"mini-container/internal/container"
	logpkg "mini-container/internal/log"
	"mini-container/internal/namespace"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultAPI = "http://127.0.0.1:8080/api/v1"

type client struct {
	base string
	http *http.Client
}

func newClient(base string) *client {
	return &client{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 6 * time.Minute}}
}
func (c *client) request(method, path string, body io.Reader, content string) ([]byte, error) {
	r, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		return nil, err
	}
	if content != "" {
		r.Header.Set("Content-Type", content)
	}
	res, err := c.http.Do(r)
	if err != nil {
		return nil, fmt.Errorf("daemon request failed: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("daemon returned %s: %s", res.Status, strings.TrimSpace(string(data)))
	}
	return data, nil
}
func jsonBody(v any) (*bytes.Reader, error) {
	data, err := json.Marshal(v)
	return bytes.NewReader(data), err
}
func printJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		_, err = os.Stdout.Write(data)
		return err
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		fmt.Println(string(out))
	}
	return err
}
func daemon(args []string) error {
	f := flag.NewFlagSet("daemon", flag.ContinueOnError)
	state := f.String("state", "./state", "runtime state directory")
	listen := f.String("listen", ":8080", "HTTP listen address")
	web := f.String("web", "", "web asset directory")
	if err := f.Parse(args); err != nil {
		return err
	}
	m, err := container.NewManager(*state)
	if err != nil {
		return err
	}
	logger := logpkg.New(os.Stderr, logpkg.Info)
	dir := api.ResolveWebDir(*web)
	if err = api.ValidateWebDir(dir); err != nil {
		return err
	}
	return api.NewServer(m, logger, dir).Listen(*listen)
}
func create(c *client, args []string, start bool) error {
	f := flag.NewFlagSet("create", flag.ContinueOnError)
	name := f.String("name", "", "")
	img := f.String("image", "", "")
	cmd := f.String("cmd", "/bin/sh", "")
	host := f.String("hostname", "", "")
	netmode := f.String("network", "bridge", "")
	shares := f.Int64("cpu-shares", 1024, "")
	quota := f.Float64("cpu-quota", 0, "")
	mem := f.Int64("memory", 0, "")
	pids := f.Int64("pids-limit", 0, "")
	auto := f.Bool("rm", false, "")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *img == "" {
		return fmt.Errorf("--image is required")
	}
	body, _ := jsonBody(api.CreateContainerReq{Name: *name, Image: *img, Cmd: []string{"/bin/sh", "-c", *cmd}, Hostname: *host, NetworkMode: *netmode, Resources: container.Resources{CPUShares: *shares, CPUQuota: *quota, MemoryMB: *mem, PidsLimit: *pids}, AutoRemove: *auto})
	data, err := c.request("POST", "/containers", body, "application/json")
	if err != nil {
		return err
	}
	if !start {
		return printJSON(data)
	}
	var item api.ContainerView
	if err = json.Unmarshal(data, &item); err != nil {
		return err
	}
	data, err = c.request("POST", "/containers/"+item.ID+"/start", nil, "")
	if err != nil {
		return err
	}
	return printJSON(data)
}
func simple(c *client, method, path string) error {
	data, err := c.request(method, path, nil, "")
	if err != nil {
		return err
	}
	if len(data) > 0 {
		return printJSON(data)
	}
	return nil
}
func execCommand(c *client, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: mini-container exec CONTAINER COMMAND [ARG...]")
	}
	body, _ := jsonBody(api.ExecReq{Cmd: args[1:]})
	data, err := c.request("POST", "/containers/"+args[0]+"/exec", body, "application/json")
	if err == nil {
		_, err = os.Stdout.Write(data)
	}
	return err
}
func importImage(c *client, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return err
	}
	if _, err = io.Copy(part, file); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	data, err := c.request("POST", "/images/import", &body, w.FormDataContentType())
	if err != nil {
		return err
	}
	return printJSON(data)
}
func exportImage(c *client, name, path string) error {
	data, err := c.request("GET", "/images/"+name+"/export", nil, "")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("command is required")
	}
	if args[0] == "__init" {
		if len(args) != 2 {
			return fmt.Errorf("invalid init invocation")
		}
		return namespace.RunInit(args[1])
	}
	if args[0] == "daemon" {
		return daemon(args[1:])
	}
	base := os.Getenv("MINI_CONTAINER_API")
	if base == "" {
		base = defaultAPI
	}
	c := newClient(base)
	switch args[0] {
	case "create":
		return create(c, args[1:], false)
	case "run":
		return create(c, args[1:], true)
	case "ps":
		return simple(c, "GET", "/containers")
	case "images":
		return simple(c, "GET", "/images")
	case "inspect":
		if len(args) != 2 {
			return fmt.Errorf("inspect requires ID")
		}
		return simple(c, "GET", "/containers/"+args[1])
	case "start":
		if len(args) != 2 {
			return fmt.Errorf("start requires ID")
		}
		return simple(c, "POST", "/containers/"+args[1]+"/start")
	case "stop":
		if len(args) != 2 {
			return fmt.Errorf("stop requires ID")
		}
		return simple(c, "POST", "/containers/"+args[1]+"/stop")
	case "rm":
		if len(args) != 2 {
			return fmt.Errorf("rm requires ID")
		}
		return simple(c, "DELETE", "/containers/"+args[1])
	case "logs":
		if len(args) != 2 {
			return fmt.Errorf("logs requires ID")
		}
		data, err := c.request("GET", "/containers/"+args[1]+"/logs?tail=200", nil, "")
		if err == nil {
			_, err = os.Stdout.Write(data)
		}
		return err
	case "stats":
		if len(args) != 2 {
			return fmt.Errorf("stats requires ID")
		}
		return simple(c, "GET", "/containers/"+args[1]+"/stats")
	case "exec":
		return execCommand(c, args[1:])
	case "import":
		if len(args) != 2 {
			return fmt.Errorf("import requires tar file")
		}
		return importImage(c, args[1])
	case "export":
		if len(args) != 3 {
			return fmt.Errorf("export requires IMAGE FILE")
		}
		return exportImage(c, args[1], args[2])
	case "rmi":
		if len(args) != 2 {
			return fmt.Errorf("rmi requires IMAGE")
		}
		return simple(c, "DELETE", "/images/"+args[1])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
