# AGENTS.md

barista：面向多仓库工作区的开发环境管理 CLI（Java 生态优先，JDK/Maven 管理在路线图首位，git 批量操作为基础能力）。Go 实现，调用系统 git（禁止 go-git），CLI 用 cobra + charmbracelet/fang，TUI 用 bubbletea + lipgloss。

## 构建与验证

```bash
go build ./... && go vet ./...   # 必须通过，gofmt 干净
./build.sh                       # 交叉编译到 dist/
```

纯逻辑配单元测试（stdlib `testing`，与被测代码同包，`go test ./...` 必须全绿）；CLI 端到端行为靠离线 e2e（临时目录建 bare 仓库作为 `file://` 远端 + `.barista/` 工作区，脚本放 `$TEMP`，不进仓库）。

## 架构与分层契约

```txt
cmd/barista            入口，只做 fang.Execute
internal/cli/       命令层：解析参数、组装 []Task；不碰业务逻辑
internal/workspace/ 工作区发现（向上找 .barista/）、repos.json / config.json 加载校验
internal/gitrun/    git 域：exec 封装、单仓库操作、默认分支解析
internal/runner/    通用并发 worker pool（泛型，不绑定 git 语义）
internal/output/    Result 类型 + text / json / tui 三种 renderer
```

核心契约：**cli 产出 `[]Task` → runner 产出 `[]Result` → renderer 只消费 Result**。renderer 不得触碰 git 逻辑。新增功能域（jdk、tool）时各自实现 Task，runner 和 output 不得为此修改。

## 输出契约（改动前必读，面向 AI Agent 设计）

- stdout/stderr 严格分离：结果（含 JSON）走 stdout，诊断走 stderr
- JSON 输出带 `schemaVersion: 1`；`results` 按 repos.json 声明顺序（确定性，不随并发漂移）
- 错误用机器可读 code，枚举定义在 `internal/output/result.go`，新增场景应加新码而不是复用近似码
- exit code：0 全成功 / 1 任一仓库 failed / 2 用法、配置、工作区错误
- 非 TTY 自动降级：无 TUI、无颜色（遵守 `NO_COLOR`）；`--json` 隐含这一切
- **非 TTY 下永不阻塞等待输入**。本该询问的场景以 `CONFIRMATION_REQUIRED`（exit 2，带 hint + affected）报错；每个交互点必须有对应 flag 或 `--yes` 通路
- 文本输出高亮语义集中在 `internal/output/color.go`（palette 样式函数 + 启用判定），新命令复用同一套样式函数，不得在命令实现里手写 ANSI 码
- `barista schema` 输出内嵌 JSON Schema（自描述能力）；schema 单一数据源在 `internal/schema/`（go:embed），`schema validate` 也以它为校验真相（santhosh-tekuri/jsonschema 编译内嵌 schema）；新增配置文件域时同步添加 schema 文件并 embed

## 配置分层

- `<workspace>/.barista/repos.json`：仓库清单（事实）+ `config` map（git 操作行为默认值）
- `<workspace>/.barista/config.json`：全局个人设置（parallel、color）
- 优先级（低→高）：内置默认 → config.json → repos.json config → 命令行 flag；bool flag 用 `cmd.Flags().Changed()` 判断是否显式设置
- 归属判定规则：**客观事实**（baseUrl、defaultBranch、repos）放 repos.json 顶层字段；**行为偏好**（pull.rebase、fetch.prune）放 config map。拿不准时按此规则裁决
- 所有层级对未知字段宽容（忽略）

## 行为规则

- 一切操作幂等：重复执行不产生额外副作用；已满足终态的仓库报 `skipped` 并带原因码
- 默认分支解析链（`internal/gitrun/branch.go`）：repo → top → origin-head → probe(main/master/trunk)，**逐级回退**——候选分支在该仓库无真实 ref 时落到下一级，而不是失败；全落空才报 `DEFAULT_BRANCH_UNRESOLVED`
- checkout 新建分支必须 `--no-track`（否则 push.default=simple 推不上去）；push 在无 upstream 时自动 `-u origin <branch>`
- status 不静默联网，ahead/behind 基于本地缓存的远端引用

## 编码约定

- 不写解释性注释，不主动创建文档文件
- CLI 输出文案用英文
