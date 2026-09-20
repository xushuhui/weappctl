# 面向非工程同事的只读 MCP 服务

产品与运营同事需要小程序短剧数据，但他们的处境和 weappctl 原本面向的使用者不同：他们用 VS Code 里的 Codex 提问题，不会敲命令行、不会装 Go 工具链、不会维护 `~/.weappctl/config.yaml` 里的 appid/secret，也**没有能力发现 agent 答错了**。同时 weappctl 原来的输出形态也不适合 agent：`drama list` 全量是 1150 条、约 3 MB（单条约 2.5 KB，≈600 token），而"审核中"该看 `audit_detail.status` 而不是 `DramaInfo.status` 这个陷阱，连微信控制台的口径都和原始字段不一致。

因此本次不把 weappctl 直接交给同事，而是**在一台内网机器上跑一个只读 MCP 服务**，把领域语义编码进工具契约，同事侧只需要一次配置。

## 决定

1. **形态：只读 MCP 服务（streamable HTTP）**，暴露且只暴露 3 个工具：`list_dramas`、`published_dramas`、`get_drama`，全部标注 `readOnlyHint`，并将 `destructiveHint` 显式置为 false（MCP 规范里它默认为 true，而 Codex 对声明了破坏性的工具每次调用都强制审批）。
2. **鉴权：每人一个 bearer token**，通过 Codex 侧 `config.toml` 的 `http_headers` 直接写死 `Authorization` 头。服务端按 token 记名，可单独吊销；日志按人可读。
3. **网络：内网固定 IP 上的明文 HTTP**，默认只监听 `127.0.0.1:8787`，必须显式 `--listen <内网IP>:8787` 才对外。appid/secret 只存在于服务端。
4. **领域语义进契约**：状态枚举（`invalid / in-review / rejected / approved / returned / taken-down`）写进 input schema，`audit_status`（权威审核阶段）与 `taken_down`（平台下架）作为两个正交字段输出，"上架 ≠ 审核通过"写进工具 description。输出一律是信封：`matched` / `returned` / `truncated` / `counts` / `dramas`，默认只投影 4 个字段、最多 200 条。
5. **新鲜度：每次实时查微信，不做缓存。** 触发加缓存的条件：日志显示微信限流，或单日调用超过 5000 次。
6. **构建在开发机上完成**：交叉编译出 linux/amd64 二进制，Linux 机器上不需要 Go、也不需要 just。

## 被否方案与理由

| 被否方案 | 否掉的理由 |
| --- | --- |
| SKILL.md（把领域知识写成技能，agent 自己跑 CLI） | 要求 agent 用 shell 执行 `weappctl` 并解析输出，而 Codex 默认沙箱**关闭命令网络**，非技术同事会撞上看不懂的联网审批或静默失败；契约是散文，flag 改动后必然漂移 |
| 本地 stdio MCP（每人装二进制） | 把二进制、appid/secret、升级三件事全推给了最不该承担的人；VS Code 从 Dock 启动读不到 `~/.zshrc`，连 token 环境变量都会静默失效 |
| `bearer_token_env_var`（远程 + 环境变量传 token） | 同一个环境变量继承问题；改用 `http_headers` 后连 OAuth 都不必实现 |
| OAuth 登录（`codex mcp login`） | 服务端要多写注册/授权码/PKCE 和同意页，而 Codex 侧该流程还在 experimental flag 之后；换来的只是省掉一次粘贴 |
| 共享一个 token | 无法单独吊销，日志分不清人，一个人泄漏等于全员重配 |
| 手写 JSON-RPC（不用官方 SDK） | MCP 协议仍在演进，自己实现等于自制不兼容风险，且无法对各宿主逐一验证 |
| 注册 `delete-media` 并依赖审批注解兜底 | ADR-0002 的前提是"调用方是为自己行为负责的人或确定性脚本"；agent 会幻觉 ID、会被 prompt injection 影响，这把刀不该出现在它的工具表里 |
| 服务端做快照 / TTL 缓存 | 1150 条只需 12 次微信请求，代价还没显现；先记录真实调用量，用数据决定（见"决定"第 5 条） |
| HTTPS + 域名 | 服务只在内网可达，多一张证书和一条 DNS 流程买不到实际收益；代价是 token 在内网明文传输，用每人独立 token + 只读白名单缓解 |

## 后果

- **你成为唯一的部署与升级通道。** 每加一个接口都要重新编译、分发、重启服务；这是选 C 时明确接受的代价。
- **服务端是唯一持有全权凭证的地方**，也因此是唯一的攻击面：只读工具白名单、每人独立 token、访问日志、每人每日调用上限（默认 2000）是它的全部边界。
- **明文 HTTP 只在内网/VPN 可达的前提下成立**，绝不做公网端口映射；同事在家办公若不连 VPN 就用不了。
- **"只读"是挡板不是锁。** 同事手里本来就有全权 AppSecret，weappctl 侧的限制只防 agent 的意外操作，不构成访问控制。
- **ADR-0002 保持不变。** agent 侧不存在破坏性命令，所以"破坏性操作的风险由调用方自行承担"这个前提仍然成立——保住它靠的是分发边界，而不是修改那条决策。
- 实测数据（用于校验）：`list_dramas` 在 1150 条样本上 `counts.audit_status.approved == 614`、`taken_down == 23`；**当前真实上架数尚未测量**，首次 `weappctl drama published` 的结果应当写入 README 作为对照。
