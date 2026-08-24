# mini-container

教学级 Linux 容器运行时，直接编排 namespace、cgroup、overlayfs、veth 与本地镜像层，并提供 REST API、CLI 和零构建 Web 控制台。

## 构建

```bash
go test ./...
go build -o bin/mini-container ./cmd/mini-container
tar -czf dist/mini-container.tar.gz bin/mini-container web README.md
```

macOS 可完成编译、测试并启动管理面；实际容器进程需要 Linux、root 权限，以及 `unshare`、`nsenter`、`mount`、`ip` 和 `iptables`。

## 运行

```bash
sudo ./bin/mini-container daemon --state /var/lib/mini-container --listen :8080
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
