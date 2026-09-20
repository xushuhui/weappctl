# weappctl 构建入口。
#
# 构建在你的 Mac 上完成，Linux 内网机器只需要拷过去二进制 —— 那台机器上不需要 Go，
# 也不需要 just（见 README 的部署章节）。

version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags := "-X github.com/xsh/weappctl/cmd.version=" + version

# 列出所有可用 recipe
default:
    @just --list

# 本机自用的 CLI（darwin/arm64）
build:
    go build -ldflags "{{ldflags}}" -o bin/weappctl .

# 服务端二进制（linux/amd64，内网只读 MCP 服务用）
build-linux:
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "{{ldflags}}" -o bin/weappctl-linux-amd64 .

# 全量测试
test:
    go test ./...

# 静态检查
vet:
    go vet ./...

# 就地格式化
fmt:
    gofmt -l -w .

# 只检查不修改，供 check 使用
fmt-check:
    @out="$(gofmt -l .)"; if [ -n "$out" ]; then echo "需要 gofmt："; echo "$out"; exit 1; fi

# 提交前总检：格式 + vet + 测试
check: fmt-check vet test
