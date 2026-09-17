# AGENTS.md

barista：面向多仓库工作区的开发环境管理 CLI（Java 生态优先，JDK/Maven 管理在路线图首位，git 批量操作为基础能力）。Go 实现，调用系统 git（禁止 go-git），CLI 用 cobra + charmbracelet/fang，TUI 用 bubbletea + lipgloss。

## 构建与验证

```bash
go build ./... && go vet ./...   # 必须通过，gofmt 干净
./build.sh                       # 交叉编译到 dist/
```

纯逻辑配单元测试（stdlib `testing`，与被测代码同包，`go test ./...` 必须全绿）；CLI 端到端行为靠离线 e2e（临时目录建 bare 仓库作为 `file://` 远端 + `.barista/` 工作区，脚本放 `$TEMP`，不进仓库）。

## 目录结构

```txt
cmd/barista/          入口（fang.Execute）
internal/
  cli/              cobra 命令层：root（--json/--parallel）+ git/ 命令组 + schema.go
  workspace/        工作区发现（向上找 .barista/）、repos.json 与两级 config.json 加载合并
  gitrun/           git 域：exec 封装、单仓库操作、默认分支解析链
  runner/           通用并发 worker pool（泛型，不绑定 git 语义）
  output/           Result 类型 + text/json/tui renderer + 高亮（color.go）
schemas/            JSON Schema 单一数据源（包即数据目录，同目录 go:embed）
docs/               架构与契约文档
build.sh            交叉编译六平台（--install 装到 ~/.local/bin）
```

## 关键契约（速览）

完整版见 [docs/architecture.md](docs/architecture.md)，改动对应区域前必读。

- 分层：cli 产出 `[]Task` → runner 产出 `[]Result` → renderer 只消费 Result；renderer 不碰 git 逻辑
- 输出：stdout/stderr 严格分离；JSON 带 `schemaVersion: 1`、机器可读错误码、按声明顺序；exit 0/1/2；非 TTY 永不阻塞询问
- 高亮：样式函数集中在 `internal/output/color.go`，不得手写 ANSI
- schema：单一数据源在 `schemas/`，新配置域同步加 JSON 文件并 embed
- 配置优先级：内置默认 → user（`~/.config/barista/config.json`）→ workspace（`.barista/config.json`）→ repos.json config → flag；未知字段宽容
- 行为：一切操作幂等；checkout 新分支 `--no-track`，push 无 upstream 自动 `-u`；status 不静默联网

## 编码约定

- 不写解释性注释，不主动创建文档文件
- CLI 输出文案用英文
