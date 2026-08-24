基于 Go 实现的简易容器运行时项目，一款 Linux 容器运行时工具，支持 namespace 进程隔离、cgroup 资源限制、overlayfs 镜像分层与 REST API 管理。

# mini-container

教学级 Linux 容器运行时，直接编排 namespace、cgroup、overlayfs、veth 与本地镜像层，并提供 REST API、CLI 和零构建 Web 控制台。

## 构建与测试

```bash
go mod download
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

构建评测镜像（支持 Linux arm64 与 amd64）：

```bash
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh mini-container linux/arm64
./build_benzhi_docker.sh mini-container linux/amd64
```

## 运行

```bash
go build -o bin/mini-container ./cmd/mini-container
sudo ./bin/mini-container daemon --state /var/lib/mini-container --listen :8080 --web ./web
```

浏览器打开 `http://127.0.0.1:8080`。CLI 默认连接该地址，也可用 `MINI_CONTAINER_API` 指定 API 根地址。

```bash
./bin/mini-container import busybox.tar
./bin/mini-container run --name demo --image busybox:latest --cmd "sleep 3600"
./bin/mini-container ps
./bin/mini-container exec demo /bin/hostname
./bin/mini-container stop demo
./bin/mini-container rm demo
```

镜像 tar 必须在根目录包含 `manifest.json`，以及 manifest 中列出的层目录：

```json
{"name":"busybox","tag":"latest","layers":["layer0"]}
```

实际容器进程需要 Linux、root 权限，以及 `unshare`、`nsenter`、`mount`、`ip` 和 `iptables`；macOS 可完成编译、测试和管理面启动，但不能运行 Linux namespace 容器。
