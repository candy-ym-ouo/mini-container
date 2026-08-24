# 官方多架构 Go 镜像，保留完整工具链供离线评测使用。
FROM golang:1.26

WORKDIR /app

# 项目当前无第三方依赖；保留 go.mod 并预热模块缓存。
COPY go.mod ./
RUN go mod download

COPY . .

# 在镜像构建阶段确认整个项目可编译。
RUN go build ./...

CMD ["bash"]
