# 架构与契约

barista 的目录结构与设计契约，改动代码前必读。

## 目录结构

```txt
cmd/barista/          入口（fang.Execute）
internal/
  cli/              cobra 命令层：root（--json/--parallel）+ git/ 命令组 + jdk/ 命令组 + maven/ 命令组 + schema.go
  workspace/        工作区发现（向上找 .barista/）、repos.json 与两级 config.json 加载合并、properties 写入
  gitrun/           git 域：exec 封装、单仓库操作、默认分支解析链
  jdk/              JDK 域：registry（~/.barista/jdk.json）读写、java 探测（version/distro）、版本比较
  maven/            Maven 域：registry（~/.barista/maven.json）读写、文件系统探测（maven-core jar）、版本比较、模糊解析
  download/         通用下载（断点续传/退避重试）与归档解压（zip/tar.gz），jdk 与 maven 共用
  runner/           通用并发 worker pool（泛型，不绑定 git 语义）
  output/           Result 类型 + text/json/tui renderer + 高亮（color.go）+ 下载进度格式化（progress.go）
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
internal/maven/     Maven 域：registry 读写（原子写）、probe（纯文件系统：bin/mvn 存在性 + lib/maven-core-*.jar 文件名解析版本，不起子进程）、版本比较（数字段 + qualifier token 比较，ComparableVersion 简化版）、模糊解析、discover 候选收集（MAVEN_HOME/M2_HOME / sdkman / brew / scoop / 平台位置 / PATH 反推）
internal/download/  通用下载（Range 断点续传、指数退避重试、4xx 不重试）与归档解压（剥首层、防 zip-slip/逃逸 symlink）
internal/runner/    通用并发 worker pool（泛型，不绑定 git 语义）
internal/output/    Result 类型 + text / json / tui 三种 renderer
schemas/            JSON Schema 单一数据源（包即数据目录，同目录 go:embed）
```

核心契约：**cli 产出 `[]Task` → runner 产出 `[]Result` → renderer 只消费 Result**。renderer 不得触碰 git 逻辑。新增功能域（jdk、tool）时各自实现 Task，runner 和 output 不得为此修改。

jdk 命令组的特殊性：registry 是 user 级文件，命令不依赖工作区，故 cli/jdk 自带执行骨架（不调用 workspace.Load）；output 的 TextRenderer 绑定 git 语义（"repos" 汇总行、git 动作描述），jdk 域文本输出在 cli/jdk 内自行排版（tabwriter），色彩复用导出的 `output.Palette`（高亮语义仍集中在 color.go，命令实现不手写 ANSI），JSON 仍走 output.Envelope。discover 是 jdk 域唯一走 runner 并发的命令（每个候选路径一个 Task，probe 起子进程）；注册在主线程串行进行，命名按 `<distro><major>` 冲突追加 `-1`/`-2`。discover 幂等可重复：已注册路径（SamePath 比较）报 skipped，原因放 `Detail.reason`（不带 Error，重复跑不产生错误）；probe 失败的候选同样 skipped 但保留 Error 诊断。install 通过 Provider 接口解析发行版下载地址（当前仅 temurin → Adoptium API），下载支持断点续传（Range 头）与指数退避重试（默认 4 次，4xx 不重试），TTY 下 stderr 渲染进度条（百分比/速度/ETA）；下载后解压到 `<installDir>/<name>`（剥归档首层目录），probe 校验 major 匹配才注册（`managed: true`），任何失败清理半成品目录。uninstall 仅作用于 managed 条目，删除 `<installDir>/<name>` 整树并注销（级联清 defaults）；确认契约：TTY 交互询问，非 TTY 报 CONFIRMATION_REQUIRED（exit 2），`--yes` 直通。

maven 命令组镜像 jdk 的骨架与契约（自带执行骨架、tabwriter 文本、Envelope JSON、install/uninstall 同样的半成品清理与确认契约），差异点：registry 为 `~/.barista/maven.json`（`installations` + `default` + `jdk`）；probe 是纯文件系统探测（校验 `bin/mvn`/`bin/mvn.cmd` 存在、从 `lib/maven-core-*.jar` 文件名解析版本），不起子进程，因此 discover 串行不走 runner，也不依赖 java 可用。自动命名为 `maven-<major.minor>`（版本号保留两段，qualifier 不进名字），add/discover/install 共用，冲突走 AvailableName 追加 `-1`/`-2`；`add <path> [--name]`（--name 禁止形似版本号，避免与模糊解析歧义）。解析（which/path/home，三命令独立可见）：精确名字 → 精确版本 → 数字段前缀逐级扩大（`3.9.11`→`3.9.*`→`3.*`）取 CompareVersions 最大者 → MAVEN_NOT_FOUND；模糊解析只用于只读解析，remove/uninstall/set-default 强制精确名字。install 单 Provider（Apache archive：`archive.apache.org/dist/maven/maven-<major>/<version>/`），一律 tar.gz，下载后强制 sha512 校验（`.sha512` 旁挂文件，不匹配报 MAVEN_CHECKSUM_MISMATCH）。`maven config` 内省命令打印合并后的有效配置及来源（source 枚举：workspace/user/builtin/ambient/none）。

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

user 级统一目录 `~/.barista/`（全平台一致，`os.UserHomeDir()` + `.barista`，与 workspace 级 `.barista/` 对应）：

```txt
~/.barista/
  config.json          用户配置（不存在合法，语法错误报 CONFIG_ERROR）
  jdk.json             JDK registry（jdks 数组 + defaults major→name；不存在视为空注册表；临时文件 + rename 原子写）
  maven.json           Maven registry（installations 数组 + default + jdk；不存在视为空注册表；原子写同上）
  toolchains/          barista 托管安装的工具链（仅 install 产物；add/discover 注册的可在任意路径）
    jdk/<name>/        barista jdk install 安装根（JDK 域默认 installDir）
    maven/<name>/      barista maven install 安装根（Maven 域默认 mavenInstallDir）
```

- config.json `installDir`：`barista jdk install` 的安装根目录（默认 `~/.barista/toolchains/jdk`），仅 user level 生效，不参与 workspace 合并
- config.json `mavenInstallDir`：`barista maven install` 的安装根目录（默认 `~/.barista/toolchains/maven`），同上仅 user level
- config.json `properties`：自由 KV map（点分键，Java properties 风格）。`maven.*` 键仅 workspace level 被读取（user level 写了也忽略）；写入走 `workspace.SetConfigProperty`（通用 map 读写，未知字段全保留）
- maven 域偏好分层：user 级唯一来源是 maven.json 的 `default`/`jdk` 字段；workspace 级覆盖放 `<workspace>/.barista/config.json` 的 `properties["maven.default"]` / `properties["maven.jdk"]`，由 `barista maven set-default/set-jdk --scope workspace` 写入。解析链：`--flag > workspace properties > maven.json > 缺省（default 报 MAVEN_NOT_FOUND；jdk 回退环境原样）`。workspace 覆盖解析不到目标时响着报错，不静默回退
- maven 命令的 workspace 发现是机会主义的（FindRoot 找不到不算错误，仅意味着无 workspace 级覆盖）；**home 目录守卫**：向上找到的 `.barista` 若就是 user 级 `~/.barista`（root == 用户主目录），不视为 workspace——防止把 workspace 偏好写进 user 级 config.json
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
