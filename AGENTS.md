# AGENTS.md

## 项目

barista：面向多仓库工作区的开发环境管理 CLI（Java 生态优先，JDK/Maven 管理在路线图首位，git 批量操作为基础能力）。

| 项   | 技术选型                    |
| ---- | --------------------------- |
| 语言 | Go 1.26+                    |
| git  | 调用系统 git（禁止 go-git） |
| CLI  | cobra + charmbracelet/fang  |
| TUI  | bubbletea + lipgloss        |

## 构建与验证

| 命令                             | 要求                                                    |
| -------------------------------- | ------------------------------------------------------- |
| `go build ./... && go vet ./...` | 必须通过                                                |
| `golangci-lint run ./...`        | 0 issues（配置 [.golangci.yml](.golangci.yml)，v2.13.x） |
| `go test ./...`                  | 全绿；纯逻辑配单元测试（stdlib `testing`，同包）        |
| `./build.sh`                     | 交叉编译六平台到 dist/，`--install` 装到 `~/.local/bin` |

CLI 端到端行为靠离线 e2e：临时目录建 bare 仓库作为 `file://` 远端 + `.barista/` 工作区，脚本放 `$TEMP`，不进仓库。`go test` 必须全程离线（httptest / 临时目录可以）；真实网络验证只用手动 e2e，若未来确需联网测试用例，必须以环境变量门控且默认 skip，CI 不开启。

## 文档索引

- [docs/architecture.md](docs/architecture.md)——全局架构契约（跨域关注点），**改动代码前必读**
- [docs/design/](docs/design/)——各域详细设计（workspace / git / deps / jdk / maven / gradle / executors / doctor / upgrade），**改动对应域前必读**
- [docs/commands.md](docs/commands.md)——CLI 命令参考，命令/flag 变更时同步更新

## 红线

- 分层：cli 产出 `[]Task` → runner 产出 `[]Result` → renderer 只消费 Result；renderer 不碰 git 逻辑
- 输出：stdout/stderr 严格分离；JSON 带 `schemaVersion: 1`、机器可读错误码、按声明顺序；exit 0/1/2；非 TTY 永不阻塞询问
- 一切操作幂等；不写解释性注释；CLI 输出文案用英文
