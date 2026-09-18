# 架构与契约

barista 的目录结构与设计契约，改动代码前必读。

## 目录结构

```txt
cmd/barista/          入口（fang.Execute）
internal/
  cli/              cobra 命令层：root（--json/--parallel）+ git/ 命令组 + jdk/ 命令组 + schema.go
  workspace/        工作区发现（向上找 .barista/）、repos.json 与两级 config.json 加载合并
  gitrun/           git 域：exec 封装、单仓库操作、默认分支解析链
  jdk/              JDK 域：registry（~/.config/barista/jdk.json）读写、java 探测（version/distro）、版本比较
  runner/           通用并发 worker pool（泛型，不绑定 git 语义）
  output/           Result 类型 + text/json/tui renderer + 高亮（color.go）
schemas/            JSON Schema 单一数据源（包即数据目录，同目录 go:embed）
docs/               架构与契约文档
build.sh            交叉编译六平台（--install 装到 ~/.local/bin）
```

## 分层契约

```txt
cmd/barista            入口，只做 fang.Execute
internal/cli/       命令层：解析参数、组装 []Task；不碰业务逻辑
internal/workspace/ 工作区发现（向上找 .barista/）、repos.json / 两级 config.json 加载校验
internal/gitrun/    git 域：exec 封装、单仓库操作、默认分支解析
internal/jdk/       JDK 域：registry 读写（原子写）、probe（java -version / -XshowSettings 解析）、版本解析比较、discover 候选收集（JAVA_HOME 系 env / sdkman / 平台安装位置 / PATH 反推）
internal/runner/    通用并发 worker pool（泛型，不绑定 git 语义）
internal/output/    Result 类型 + text / json / tui 三种 renderer
schemas/            JSON Schema 单一数据源（包即数据目录，同目录 go:embed）
```

核心契约：**cli 产出 `[]Task` → runner 产出 `[]Result` → renderer 只消费 Result**。renderer 不得触碰 git 逻辑。新增功能域（jdk、tool）时各自实现 Task，runner 和 output 不得为此修改。

jdk 命令组的特殊性：registry 是 user 级文件，命令不依赖工作区，故 cli/jdk 自带执行骨架（不调用 workspace.Load）；output 的 TextRenderer 绑定 git 语义（"repos" 汇总行、git 动作描述），jdk 域文本输出在 cli/jdk 内自行排版（tabwriter），色彩复用导出的 `output.Palette`（高亮语义仍集中在 color.go，命令实现不手写 ANSI），JSON 仍走 output.Envelope。discover 是 jdk 域唯一走 runner 并发的命令（每个候选路径一个 Task，probe 起子进程）；注册在主线程串行进行，命名按 `<distro><major>` 冲突追加 `-1`/`-2`。discover 幂等可重复：已注册路径（SamePath 比较）报 skipped，原因放 `Detail.reason`（不带 Error，重复跑不产生错误）；probe 失败的候选同样 skipped 但保留 Error 诊断。

## 输出契约（面向 AI Agent 设计）

- stdout/stderr 严格分离：结果（含 JSON）走 stdout，诊断走 stderr
- JSON 输出带 `schemaVersion: 1`；`results` 按 repos.json 声明顺序（确定性，不随并发漂移）
- 错误用机器可读 code，枚举定义在 `internal/output/result.go`，新增场景应加新码而不是复用近似码
- exit code：0 全成功 / 1 任一仓库 failed / 2 用法、配置、工作区错误
- 非 TTY 自动降级：无 TUI、无颜色（遵守 `NO_COLOR`）；`--json` 隐含这一切
- **非 TTY 下永不阻塞等待输入**。本该询问的场景以 `CONFIRMATION_REQUIRED`（exit 2，带 hint + affected）报错；每个交互点必须有对应 flag 或 `--yes` 通路
- 文本输出高亮语义集中在 `internal/output/color.go`（palette 样式函数 + 启用判定），新命令复用同一套样式函数，不得在命令实现里手写 ANSI 码
- `barista schema` 输出内嵌 JSON Schema（自描述能力）；schema 单一数据源在 `schemas/` 目录（包即数据目录，`schemas/schemas.go` 与 JSON 同目录 go:embed），`schema validate` 也以它为校验真相（santhosh-tekuri/jsonschema 编译内嵌 schema）；新增配置文件域时同步在 `schemas/` 添加 JSON 文件并 embed

## 配置分层

- user level：`~/.config/barista/config.json`（全平台统一 XDG 形态，`os.UserHomeDir()` + `.config/barista/config.json`；不存在合法，语法错误报 CONFIG_ERROR）
- user level：`~/.config/barista/jdk.json`——JDK registry（`jdks` 数组 + `defaults` major→name）；不存在视为空注册表；写盘为临时文件 + rename 原子替换
- config.json `installDir`：`barista jdk install` 的安装根目录（默认 `~/.local/barista/jdks`），仅 user level 生效，不参与 workspace 合并
- workspace level：`<workspace>/.barista/config.json`（与 user level 同 schema，覆盖 user level）
- `<workspace>/.barista/repos.json`：仓库清单（事实）+ `config` map（git 操作行为默认值）
- 优先级（低→高）：内置默认 → user level → workspace level → repos.json config → 命令行 flag；bool flag 用 `cmd.Flags().Changed()` 判断是否显式设置
- 归属判定规则：**客观事实**（baseUrl、defaultBranch、repos）放 repos.json 顶层字段；**行为偏好**（pull.rebase、fetch.prune）放 config map。拿不准时按此规则裁决
- 所有层级对未知字段宽容（忽略）

## 行为规则

- 一切操作幂等：重复执行不产生额外副作用；已满足终态的仓库报 `skipped` 并带原因码
- 默认分支解析链（`internal/gitrun/branch.go`）：repo → top → origin-head → probe(main/master/trunk)，**逐级回退**——候选分支在该仓库无真实 ref 时落到下一级，而不是失败；全落空才报 `DEFAULT_BRANCH_UNRESOLVED`
- checkout 新建分支必须 `--no-track`（否则 push.default=simple 推不上去）；push 在无 upstream 时自动 `-u origin <branch>`
- status 不静默联网，ahead/behind 基于本地缓存的远端引用
