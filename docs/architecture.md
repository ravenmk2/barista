# 架构与契约

barista 的目录结构与设计契约，改动代码前必读。

## 目录结构

```txt
cmd/barista/          入口（fang.Execute）
internal/
  cli/              cobra 命令层：root（--json/--parallel）+ git/ 命令组 + jdk/ 命令组 + maven/ 命令组 + mvn/ 执行器命令 + schema.go
  workspace/        工作区发现（向上找 .barista/，FindWorkspaceRoot 含 home 守卫）、repos.json 与两级 config.json 加载合并、properties 写入、cwd→repo 匹配（MatchRepo 最长前缀）
  gitrun/           git 域：exec 封装、单仓库操作、默认分支解析链
  jdk/              JDK 域：registry（~/.barista/jdk.json）读写、java 探测（version/distro）、版本比较
  maven/            Maven 域：registry（~/.barista/maven.json）读写、文件系统探测（maven-core jar）、版本比较、模糊解析
  download/         通用下载（断点续传/退避重试）与归档解压（zip/tar.gz），jdk 与 maven 共用
  runner/           通用并发 worker pool（泛型，不绑定 git 语义）
  output/           Result 类型 + text/json/tui renderer + 高亮（color.go）+ 下载进度格式化（progress.go）
schemas/            JSON Schema 单一数据源（包即数据目录，同目录 go:embed）
docs/               架构契约与命令参考文档
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

jdk 命令组的特殊性：registry 是 user 级文件，命令不依赖工作区，故 cli/jdk 自带执行骨架（不调用 workspace.Load）；output 的 TextRenderer 绑定 git 语义（"repos" 汇总行、git 动作描述），jdk 域文本输出在 cli/jdk 内自行排版（tabwriter；which 例外，恒走 Envelope JSON，`--pathonly`/`path`/`home` 输出纯路径），色彩复用导出的 `output.Palette`（高亮语义仍集中在 color.go，命令实现不手写 ANSI），JSON 仍走 output.Envelope。discover 是 jdk 域唯一走 runner 并发的命令（每个候选路径一个 Task，probe 起子进程）；注册在主线程串行进行，命名按 `<distro><major>` 冲突追加 `-1`/`-2`。discover 幂等可重复：已注册路径（SamePath 比较）报 skipped，原因放 `Detail.reason`（不带 Error，重复跑不产生错误）；probe 失败的候选同样 skipped 但保留 Error 诊断。install 通过 Provider 接口解析发行版下载地址（当前仅 temurin → Adoptium API），下载支持断点续传（Range 头）与指数退避重试（默认 4 次，4xx 不重试），TTY 下 stderr 渲染进度条（百分比/速度/ETA）；下载后解压到 `<installDir>/<name>`（剥归档首层目录），probe 校验 major 匹配才注册（`managed: true`），任何失败清理半成品目录。uninstall 仅作用于 managed 条目，删除 `<installDir>/<name>` 整树并注销（级联清 defaults）；确认契约：TTY 交互询问，非 TTY 报 CONFIRMATION_REQUIRED（exit 2），`--yes` 直通。

maven 命令组镜像 jdk 的骨架与契约（自带执行骨架、tabwriter 文本、Envelope JSON、install/uninstall 同样的半成品清理与确认契约），差异点：registry 为 `~/.barista/maven.json`（`installations` + `default` + `jdk`）；probe 是纯文件系统探测（校验 `bin/mvn`/`bin/mvn.cmd` 存在、从 `lib/maven-core-*.jar` 文件名解析版本），不起子进程，因此 discover 串行不走 runner，也不依赖 java 可用。自动命名为 `maven-<major.minor>`（版本号保留两段，qualifier 不进名字），add/discover/install 共用，冲突走 AvailableName 追加 `-1`/`-2`；`add <path> [--name]`（--name 禁止形似版本号，避免与模糊解析歧义）。解析（which/path/home，三命令独立可见）：精确名字 → 精确版本 → 数字段前缀逐级扩大（`3.9.11`→`3.9.*`→`3.*`）取 CompareVersions 最大者 → MAVEN_NOT_FOUND；模糊解析只用于只读解析，remove/uninstall/set-default 强制精确名字。install 单 Provider（Apache archive：`archive.apache.org/dist/maven/maven-<major>/<version>/`），一律 tar.gz，下载后强制 sha512 校验（`.sha512` 旁挂文件，不匹配报 MAVEN_CHECKSUM_MISMATCH）。`maven config` 内省命令打印合并后的有效配置及来源（source 枚举：workspace/user/repo/builtin/ambient/none），含 cwd 命中的 repo、settings 与 repo.local 注入状态。

`barista mvn` 是**执行器**而非管理命令（cli/mvn/，不走 runner/Result 契约）：`barista mvn [flags] -- <mvn args>`，`--` 后的参数原样透传（缺 `--` 直接给 goal 报 USAGE_ERROR exit 2）。环境组装是纯函数 `planExec`（plan.go，同包单测）：maven 安装按 `workspace maven.default property > maven.json default` 解析；JDK 按 `--jdk > cwd 命中 repo 的 properties["maven.jdk"] > workspace maven.jdk > maven.json jdk > 环境原样`，显式指定的任一级解析失败响亮报 JDK_NOT_FOUND（exit 2，message 带来源），命中则只对子进程设 JAVA_HOME。注入规则：`.barista/maven/settings.xml` 存在即注入 `-s`（同目录 `settings-security.xml` 注入 `-Dsettings.security`）；workspace property `maven.repo.local` 注入 `-Dmaven.repo.local`（相对路径锚定 workspace root）；**透传参数已含 `-s`/`--settings`/对应 `-D` 时跳过该注入**（用户显式优先）。执行：Unix 直接 `bin/mvn`，Windows 经 `cmd /c bin/mvn.cmd`；stdin/stdout/stderr 直接挂父进程（流式透传，无 TUI/渲染层）；mvn 非零退出 → exit 1，barista 自身配置错误 → exit 2 + 机器可读码；`--json` 只影响 exec 前的错误（ErrorEnvelope）与 --dry-run 输出，exec 后输出归属 mvn。workspace 发现是机会主义的（无 workspace = 跳过 repo/workspace 两级覆盖，纯 user 级默认透传）。

启动模式（launch.go）：`--startup` > repo `properties["maven.startup"]` > workspace `properties["maven.startup"]` > 默认 `script`；非法值报 CONFIG_ERROR。`script` 即上述包装脚本路径。`jar` 绕过包装脚本直启 java（消除 `cmd /c` 二次解析对 `%`/`&`/`|` 参数的变形风险，双平台同一代码路径），复刻 mvn 脚本契约（3.6–3.9 实测同构）：java 取 JDK 路径的 `bin/java`（ambient 时 PATH 查找）、glob `boot/plexus-classworlds-*.jar` 恰一个 + `bin/m2.conf` 存在（否则 MAVEN_EXEC_FAILED，hint 回退 `--startup script`）、basedir 上爬 `.mvn`（感知 `-f`/`--file`，`MAVEN_BASEDIR` env 优先，落空回 cwd）、读 `.mvn/jvm.config`、透传 `MAVEN_OPTS`/`MAVEN_DEBUG_OPTS`、`MAVEN_ARGS` 仅 ≥3.9 追加、`-Dlibrary.jansi.path` 在 `lib/jansi-native` 存在时设置。差异声明：jar 模式不执行 `mavenrc_pre/post` 钩子。

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
  jdk.json             JDK registry（jdks 数组 + defaults major→name + installDir；不存在视为空注册表；临时文件 + rename 原子写）
  maven.json           Maven registry（installations 数组 + default + jdk + installDir；不存在视为空注册表；原子写同上）
  toolchains/          barista 托管安装的工具链（仅 install 产物；add/discover 注册的可在任意路径）
    jdk/<name>/        barista jdk install 安装根（JDK 域缺省 installDir）
    maven/<name>/      barista maven install 安装根（Maven 域缺省 installDir）
```

- installDir：归属各自 registry 顶层字段（jdk.json / maven.json），由 `barista jdk set-install-dir` / `barista maven set-install-dir` 写入（须为绝对路径，`~` 允许；`--reset` 清空字段恢复内置默认 `~/.barista/toolchains/jdk|maven`）；不参与 workspace 合并。旧 config.json 残留的 `installDir`/`mavenInstallDir` 已废弃，按未知字段宽容忽略
- config.json 顶层字段只剩 `parallel`/`color`/`properties`
- config.json `properties`：自由 KV map（点分键，Java properties 风格）。`maven.*` 键仅 workspace level 被读取（user level 写了也忽略）；写入走 `workspace.SetConfigProperty`（通用 map 读写，未知字段全保留）
- maven 域偏好分层：user 级唯一来源是 maven.json 的 `default`/`jdk` 字段；workspace 级覆盖放 `<workspace>/.barista/config.json` 的 `properties["maven.default"]` / `properties["maven.jdk"]`，由 `barista maven set-default/set-jdk --scope workspace` 写入。解析链：`--flag > workspace properties > maven.json > 缺省（default 报 MAVEN_NOT_FOUND；jdk 回退环境原样）`。workspace 覆盖解析不到目标时响亮报错，不静默回退
- `maven.repo.local`：workspace properties 键，`barista mvn` 显式设置时注入 `-Dmaven.repo.local`（相对路径锚定 workspace root）；CLI 注入优先级高于 settings.xml 的 `<localRepository>`，两者同设时 property 赢
- `<workspace>/.barista/maven/`：maven 执行约定目录——`settings.xml` 存在即被 `barista mvn` 以 `-s` 注入（零配置私有 settings），`settings-security.xml` 同理注入 `-Dsettings.security`
- maven 命令的 workspace 发现是机会主义的（FindRoot 找不到不算错误，仅意味着无 workspace 级覆盖）；**home 目录守卫**：向上找到的 `.barista` 若就是 user 级 `~/.barista`（root == 用户主目录），不视为 workspace——防止把 workspace 偏好写进 user 级 config.json。守卫实现收敛在 `workspace.FindWorkspaceRoot`，maven 组与 mvn 命令共用
- workspace level：`<workspace>/.barista/config.json`（与 user level 同 schema，覆盖 user level）
- `<workspace>/.barista/repos.json`：仓库清单（事实）+ `config` map（git 操作行为默认值）；repo 条目另有 `properties` map（string→string，点分域前缀键），承载 **per-repo 覆盖**——当前被读取的键：`maven.jdk`（cwd 命中 repo 时优先于 workspace maven.jdk）、`maven.startup`（jar|script，同理）；未知键宽容忽略，其他域可复用同一容器
- 优先级（低→高）：内置默认 → user level → workspace level → repo properties（per-repo 值）→ repos.json config（git 行为默认值，全 repo 统一）→ 命令行 flag；bool flag 用 `cmd.Flags().Changed()` 判断是否显式设置
- 归属判定规则：**客观事实**（baseUrl、defaultBranch、repos）放 repos.json 顶层字段；**行为偏好**（pull.rebase、fetch.prune）放 config map。拿不准时按此规则裁决
- 所有层级对未知字段宽容（忽略）

## 行为规则

- 一切操作幂等：重复执行不产生额外副作用；已满足终态的仓库报 `skipped` 并带原因码
- 默认分支解析链（`internal/gitrun/branch.go`）：repo → top → origin-head → probe(main/master/trunk)，**逐级回退**——候选分支在该仓库无真实 ref 时落到下一级，而不是失败；全落空才报 `DEFAULT_BRANCH_UNRESOLVED`
- checkout 新建分支必须 `--no-track`（否则 push.default=simple 推不上去）；push 在无 upstream 时自动 `-u origin <branch>`
- status 不静默联网，ahead/behind 基于本地缓存的远端引用
