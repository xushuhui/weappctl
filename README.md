# weappctl

代表小程序开发者在服务器端调用微信小程序服务端 API 的命令行工具。当前接入：已上架短剧（`developerGetPublishedDrama`）、剧目信息（`getDrama`）、剧目列表（`listDramas`）、删除媒资（`deleteMedia`）。领域词汇见 [CONTEXT.md](CONTEXT.md)，决策记录见 [docs/adr](docs/adr)。

有两种用法：

- **本机 CLI**：你自己排查数据、做对账（本文第一节）。
- **只读 MCP 服务**：跑在内网机器上，产品/运营同事在 VS Code 的 Codex 里直接提问（本文第二节）。同事那一侧只需要配置一次，材料见 [docs/colleague-setup.md](docs/colleague-setup.md)。

---

## 一、本机 CLI

### 配置

`~/.weappctl/config.yaml`（**必须是 0600**，里面是明文 secret）：

```yaml
profiles:
  default:
    appid: wx1234567890abcdef
    secret: 0123456789abcdef0123456789abcdef
```

```console
$ chmod 600 ~/.weappctl/config.yaml
```

凭证优先级：配置文件 < `WEAPP_APPID`/`WEAPP_SECRET` 环境变量 < `--appid`/`--secret` 参数；用 `--profile` 切换 profile，每个 profile 各自缓存 access_token（`~/.weappctl/cache`，0600）。

### 命令

```console
$ just build                       # 或 go build -o bin/weappctl .
$ bin/weappctl --help
$ bin/weappctl drama list --state taken-down        # 被平台下架的剧
$ bin/weappctl drama list --state returned          # 退回待修改
$ bin/weappctl drama list --fields drama_id,name,description   # 取更多字段
$ bin/weappctl drama published                      # 已上架清单与数量
$ bin/weappctl drama get 100271                     # 单部剧完整信息
$ bin/weappctl drama delete-media 123456            # 破坏性，不可恢复，无二次确认
```

`drama list` / `drama published` 输出统一是信封：

```json
{
  "matched": 23,          // 符合条件的总数（= 过滤后的数量）
  "returned": 23,         // 本次实际返回的条数
  "truncated": false,     // returned < matched 时为 true
  "counts": {             // 全量统计，不受 --state 影响
    "audit_status": {"invalid": 0, "in-review": 2, "rejected": 369, "approved": 614, "returned": 165},
    "taken_down": 23
  },
  "dramas": [{"drama_id": 100271, "name": "好梦过长沙", "audit_status": "approved", "taken_down": true}]
}
```

### 状态怎么读（这是本项目最容易错的地方）

| 词 | 来源 | 含义 |
| --- | --- | --- |
| `audit_status: in-review` | `audit_detail.status == 1` | 审核中（**权威**口径，与微信控制台一致） |
| `audit_status: returned` | `audit_detail.status == 4` | 退回待修改 |
| `audit_status: rejected` | `audit_detail.status == 2` | 终审不通过 |
| `audit_status: approved` | `audit_detail.status == 3` | 审核通过，**只代表有资格上架** |
| `taken_down: true` | `status == 3` | 被平台下架 |

`audit_status` 与 `taken_down` 是两个正交维度，一部剧可以同时 `approved` 且 `taken_down`。原始 `status` 字段（粗粒度可播标记，微信文档口径与实测不一致）和 `audit_detail` 仍然可用 `--fields` 原样取回，但不要拿它判断审核阶段。

**"有多少短剧上架"用 `drama published`，不要用 `--state approved`**——审核通过不等于已上架。

### 实测对照

基于 1150 条快照：`counts.audit_status.approved == 614`、`taken_down == 23`。

> 待补：`weappctl drama published` 的真实上架数**尚未测量过**，首次部署后请把结果填在这里，作为 MCP 服务答案的对照基准。

---

## 二、只读 MCP 服务（给同事用）

### 它是什么

一个跑在内网机器上的只读 HTTP 服务（MCP streamable HTTP），只暴露 3 个查询工具：

| 工具 | 回答什么问题 |
| --- | --- |
| `list_dramas` | 有多少剧在某个状态、哪些剧被下架/退回，附全量统计 |
| `published_dramas` | **现在有多少短剧上架**、上架的是哪些 |
| `get_drama` | 某部剧的完整信息 |

`delete-media` **不会**注册成工具：agent 侧不该有这把刀（见 [ADR-0002](docs/adr/0002-no-client-side-confirmation-for-destructive-commands.md)、[ADR-0003](docs/adr/0003-read-only-mcp-service-for-non-engineers.md)）。

### 部署（构建在开发机上，Linux 机器只跑二进制）

```console
# 1) 开发机：交叉编译 linux/amd64
$ just build-linux
$ scp bin/weappctl-linux-amd64 <机器>:/tmp/weappctl
```

```console
# 2) 内网机器（需要 sudo）
$ sudo install -m 0755 /tmp/weappctl /usr/local/bin/weappctl
$ sudo mkdir -p /etc/weappctl
```

`/etc/weappctl/config.yaml`（0600，内容同本机 CLI 的 profile）：

```yaml
profiles:
  default:
    appid: wx1234567890abcdef
    secret: 0123456789abcdef0123456789abcdef
```

`/etc/weappctl/tokens.yaml`（0640，每人一个 token，名字只用于日志与吊销）：

```yaml
tokens:
  - name: zhang-san
    token: 3f9c...        # openssl rand -hex 32
  - name: li-si
    token: a17d...
```

```console
$ openssl rand -hex 32                       # 给新同事生成 token
$ sudo chmod 600 /etc/weappctl/config.yaml
$ sudo chmod 640 /etc/weappctl/tokens.yaml
$ sudo chown root:root /etc/weappctl/*.yaml
```

systemd：[deploy/weappctl-mcp.service](deploy/weappctl-mcp.service)（含 `EnvironmentFile=/etc/weappctl/env`，用来放代理等环境变量，不要往 unit 文件里写密钥）。

```console
$ sudo install -m 0644 deploy/weappctl-mcp.service /etc/systemd/system/
$ sudo systemctl daemon-reload
$ sudo systemctl enable --now weappctl-mcp
$ systemctl status weappctl-mcp
$ journalctl -u weappctl-mcp -f
```

冒烟测试（在服务端本机）：

```console
$ curl -sS -o /dev/null -w '%{http_code}\n' -X POST \
    -H 'Authorization: Bearer <token>' -H 'Content-Type: application/json' \
    -H 'Accept: application/json, text/event-stream' \
    -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"0"}}}' \
    http://127.0.0.1:8787/
# 200 = 通了；401 = token 不对
```

### 安全清单（部署前逐条对）

- **内网固定 IP**：服务监听地址写死内网 IP（`--listen 10.x.x.x:8787`），和网管要一个 DHCP 保留；IP 一变，所有同事的配置同时失效。
- **绝不做公网端口映射**。内网/VPN 可达就是唯一边界；同事在家办公必须连 VPN，否则用不了。
- **明文 HTTP 的代价**：token 在内网是明文传输的。所以每人独立 token（可单独吊销）、只读工具白名单、访问日志、每人每日调用上限（`--daily-call-limit`，默认 2000）。
- **密钥只留在服务端**（`/etc/weappctl/config.yaml` 或 `EnvironmentFile`），不进代码、不进仓库、不进聊天记录。
- **出网**：机器要能访问 `api.weixin.qq.com`；要走代理就在 `/etc/weappctl/env` 里写 `HTTPS_PROXY=...`（Go 的默认 transport 会读取它）。

### 运维

```console
$ journalctl -u weappctl-mcp -f                 # 每次工具调用一行：caller / tool / matched / 耗时 / 状态
$ sudo systemctl restart weappctl-mcp           # 换 token 名单或升级二进制后
```

- **吊销某人**：从 `tokens.yaml` 删掉他那条，`systemctl restart`。
- **升级**：重新 `just build-linux` → `scp` → `install` → `restart`。同事侧什么都不用改。
- **加接口**：先加 `internal/weixin` 的调用与 `internal/dramaquery` 的语义，再决定要不要注册成第 4 个只读工具；注册前想清楚"这是不是 PM 真会问的问题"。

---

## 三、开发

```console
$ just          # 列出所有 recipe
$ just check    # gofmt 检查 + go vet + go test ./...
$ just test
$ just build-linux
```

目录：

```
cmd/                 cobra 命令（serve 是 MCP 服务入口，delete-media 只在本机 CLI）
internal/weixin/     微信服务端 API 客户端（本项目唯一的 API 层）
internal/dramaquery/ 状态语义、字段投影、输出信封（CLI 与 MCP 共用同一份）
internal/mcpserver/  只读 MCP 服务（3 个工具、token 鉴权、日上限）
internal/config/     profile 配置与 token 缓存路径
deploy/              systemd unit
docs/adr/            决策记录
```

测试不访问真实微信 API：`internal/mcpserver` 的端到端测试用 `httptest` 顶替微信，并真的跑一遍 MCP 的 `initialize` → `tools/list` → `tools/call`。
