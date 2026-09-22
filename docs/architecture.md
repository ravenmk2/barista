# 架构与契约

barista 的全局架构契约，改动代码前必读。本文件只写跨域关注点（目录地图、分层、输出、配置分层、全局行为规则）；单域行为与详细设计按域拆在 [design/](design/) 下，改动对应域前同样必读（索引见文末）。

## 目录结构

```txt
cmd/barista/          入口（fang.Execute）
internal/
  cli/              cobra 命令层：解析参数、组装 []Task，不碰业务逻辑；root（--json/--parallel）+ comp/ 补全 + 各命令组子包（init/ git/ repo/ deps/ jdk/ maven/ mvn/ java/ gradle/ node/ doctor/ upgrade/ uv/）+ schema 命令
  workspace/        工作区发现与 .barista/ 各文件读写（repos.json / config.json / properties.json）
  gitrun/           git 域：exec 封装、单仓库操作、默认分支解析
  deps/             依赖图域：建图、环检测、构建层级（纯函数）
  jdk/              JDK 域：registry、probe、版本解析、install provider
  maven/            Maven 域：registry、probe、版本解析、install provider
  gradle/           Gradle 域：registry、probe、版本解析、install provider
  node/             Node.js 域：registry、probe（起子进程 node --version）、available/install（nodejs.org dist）
  toolversion/      点分数字版本 + qualifier 的解析与比较公共包（maven / gradle / node 共用）
  download/         通用下载（断点续传 / 重试）与归档解压（zip / tar.gz），jdk / maven / gradle / node / upgrade 共用
  upgrade/          自更新域：manifest、校验、可执行文件自替换
  uv/               uv 域：轻量安装器（astral-sh/uv 资产解析、校验、解压到 ~/.local/bin）
  runner/           通用并发 worker pool（泛型，不绑定 git 语义）
  output/           Result 类型 + text / json / tui renderer + 高亮（color.go）+ TTY 判定 + 下载进度格式化（progress.go）
schemas/            JSON Schema 单一数据源（包即数据目录，同目录 go:embed）
docs/               文档（架构契约、命令参考、design/ 域详细设计）
scripts/            release 清单生成（genmanifest.py）、JDK 嵌入数据生成（gendistros.py）
.github/workflows/  test.yml（CI）/ release.yml（发布）
dist/               构建产物（build.sh 输出）
build.sh            交叉编译六平台（--install 装到 ~/.local/bin）
```

## 分层契约

核心契约：**cli 产出 `[]Task` → runner 产出 `[]Result` → renderer 只消费 Result**。renderer 不得触碰 git 逻辑。新增功能域（jdk、tool）时各自实现 Task，runner 和 output 不得为此修改。

## 输出契约（面向 AI Agent 设计）

- stdout/stderr 严格分离：结果（含 JSON）走 stdout，诊断走 stderr
- JSON 输出带 `schemaVersion: 1`；`results` 按 repos.json 声明顺序（确定性，不随并发漂移）
- 错误用机器可读 code，枚举定义在 `internal/output/result.go`，新增场景应加新码而不是复用近似码
- exit code：0 全成功 / 1 任一检查/操作 failed（jdk/maven/gradle 解析失败、upgrade 网络/校验/替换失败、schema 校验不通过、mvn/java/gradle 子进程非零等） / 2 用法、配置、工作区错误
- 非 TTY 自动降级：无 TUI、无颜色（遵守 `NO_COLOR`）；`--json` 隐含这一切
- **非 TTY 下永不阻塞等待输入**。本该询问的场景以 `CONFIRMATION_REQUIRED`（exit 2，带 hint + affected）报错；每个交互点必须有对应 flag 或 `--yes` 通路
- 文本输出高亮语义集中在 `internal/output/color.go`（Palette 样式函数 + 启用判定）：色板以 24-bit hex 定义，按 colorprofile 自动降级（truecolor→256→16，且 16 色下六个语义色落点互不相同），色深由 config `colorProfile` 控制；色板分 dark/light 两套，色系取自 charmtone（与 fang `--help` 的 DefaultColorScheme 同源），背景明暗检测逻辑与 fang 一致（真实终端查询 OSC 11，否则按 light），保证命令输出与 `--help` 同色；新命令复用同一套样式函数，不得在命令实现里手写 ANSI 码
- `barista schema` 输出内嵌 JSON Schema（自描述能力）；schema 单一数据源在 `schemas/` 目录（包即数据目录，`schemas/schemas.go` 与 JSON 同目录 go:embed），`schema validate` 也以它为校验真相（santhosh-tekuri/jsonschema 编译内嵌 schema）；新增配置文件域时同步在 `schemas/` 添加 JSON 文件并 embed

## 配置分层

分层模型与优先级是全局契约；逐文件、逐键细节见 [design/workspace.md](design/workspace.md)。

user 级统一目录 `~/.barista/`（全平台一致，`os.UserHomeDir()` + `.barista`，与 workspace 级 `.barista/` 对应）：

```txt
~/.barista/
  config.json          用户配置（不存在合法，语法错误报 CONFIG_ERROR）
  jdk.json             JDK registry（jdks 数组 + defaults major→name + installDir；不存在视为空注册表；临时文件 + rename 原子写）
  maven.json           Maven registry（installations 数组 + default + jdk + installDir；不存在视为空注册表；原子写同上）
  gradle.json          Gradle registry（同 maven.json 结构：installations + default + jdk + installDir；原子写同上）
  node.json            Node.js registry（installations + default + installDir，无 jdk 字段；原子写同上）
  toolchains/          barista 托管安装的工具链（仅 install 产物；add/discover 注册的可在任意路径）
    jdk/<name>/        barista jdk install 安装根（JDK 域缺省 installDir）
    maven/<name>/      barista maven install 安装根（Maven 域缺省 installDir）
    gradle/<name>/     barista gradle install 安装根（Gradle 域缺省 installDir）
    node/<name>/       barista node install 安装根（Node.js 域缺省 installDir）
```

- workspace 级 `<workspace>/.barista/`：`config.json`（与 user 级同 schema，覆盖 user 级）、`repos.json`（仓库清单，纯事实）、`properties.json`（workspace 级偏好 KV，仅 workspace 级存在）
- 优先级（低→高）：内置默认 → user level → workspace level（config.json + properties.json，含 git.* 键）→ repo properties（per-repo 值）→ 命令行 flag；bool flag 用 `cmd.Flags().Changed()` 判断是否显式设置
- 归属判定规则：**客观事实**（baseUrl、defaultBranch、repos）放 repos.json 顶层字段；**workspace 级行为/环境偏好**（`git.fetch.prune`、`jdk`、`node`、`maven.*`、`gradle.*` 等）放 properties.json。拿不准时按此规则裁决。键命名约定：跨域工具链声明用裸名（`jdk`、`node`），域专属偏好用点分前缀（`maven.*`、`gradle.*`、`git.*`）
- 所有层级对未知字段宽容（忽略）

## 行为规则

- 一切操作幂等：重复执行不产生额外副作用；已满足终态的仓库报 `skipped` 并带原因码
- shell 补全（cli/comp，cobra ValidArgsFunction / RegisterFlagCompletionFunc）只读本地 registry / repos.json，任何读取失败静默返回空候选，永不起子进程、不联网

## 域详细设计索引

| 文档 | 覆盖范围 |
| --- | --- |
| [design/workspace.md](design/workspace.md) | 工作区发现与 home 守卫、repos.json / properties.json / config.json 细节、init 与 repo 命令组 |
| [design/git.md](design/git.md) | gitrun 与 git 行为规则（默认分支解析链、checkout / push / status） |
| [design/deps.md](design/deps.md) | 依赖图建图、环检测、构建层级、deps 命令组 |
| [design/jdk.md](design/jdk.md) | jdk registry、probe、discover、install / download / available / uninstall |
| [design/maven.md](design/maven.md) | maven registry、probe、版本解析、install、偏好分层 |
| [design/gradle.md](design/gradle.md) | gradle registry、probe、available、install、偏好分层 |
| [design/node.md](design/node.md) | node registry、probe、available、install、mirror、use/env、偏好分层 |
| [design/executors.md](design/executors.md) | `barista mvn` / `barista java` / `barista gradle` 执行器、planExec、注入规则、启动模式 |
| [design/doctor.md](design/doctor.md) | doctor 体检项与执行模型 |
| [design/uv.md](design/uv.md) | uv 轻量安装器（无注册表，Python 委托 uv 自身） |
| [design/upgrade.md](design/upgrade.md) | 自更新流程、manifest、平台替换 |
| [design/mirror.md](design/mirror.md) | 下载镜像：分域 mirror 配置、预设映射、回退与安全契约 |
