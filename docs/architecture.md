# 架构与契约

barista 的目录结构与设计契约，改动代码前必读。

## 目录结构

```txt
cmd/barista/          入口（fang.Execute）
internal/
  cli/              cobra 命令层：root（--json/--parallel）+ comp/ 补全候选助手 + init/ 引导命令 + git/ 命令组 + repo/ 清单管理组 + deps/ 依赖图组 + jdk/ 命令组 + maven/ 命令组 + mvn/ 与 java/ 执行器命令 + doctor/ 体检命令 + upgrade/ 自更新命令 + schema.go
  workspace/        工作区发现（向上找 .barista/，FindWorkspaceRoot 含 home 守卫）、init（skeleton 创建与两层检出扫描）、repos.json 与两级 config.json 加载合并、repos.json 追加写（AddRepo）、properties.json 读写、cwd→repo 匹配（MatchRepo 最长前缀）、上爬查找（FindUpward/FindGitRoot，.git 文件/目录形态兼容）、.java-version 原子写（WriteJavaVersionFile）
  gitrun/           git 域：exec 封装、单仓库操作、默认分支解析链
  deps/             依赖图域：从 repos[].deps 声明建图，dangling 检测、SCC 环检测（Tarjan）、构建层级（最长路径 +1，环成员同层）
  jdk/              JDK 域：registry（~/.barista/jdk.json）读写、java 探测（version/distro）、版本比较、.java-version 解析（ParseJavaVersionFile/ResolveJavaVersionSpec）
  maven/            Maven 域：registry（~/.barista/maven.json）读写、文件系统探测（maven-core jar）、版本比较、模糊解析、wrapper distributionUrl 版本提取（ExtractWrapperVersion）
  download/         通用下载（断点续传/退避重试）、文件名解析（HEAD）与归档解压（zip/tar.gz），jdk 与 maven 共用
  upgrade/          自更新域：release manifest 拉取/解析/校验、版本比较、sha256 校验、可执行文件自替换（Windows rename-old）
  runner/           通用并发 worker pool（泛型，不绑定 git 语义）
  output/           Result 类型 + text/json/tui renderer + 高亮（color.go）+ TTY 判定平台实现（color_other.go/color_windows.go 的 stdoutIsTerminal）+ 下载进度格式化（progress.go）
schemas/            JSON Schema 单一数据源（包即数据目录，同目录 go:embed）
docs/               架构契约与命令参考文档
scripts/            release 清单生成（genmanifest.py）、JDK 嵌入数据生成（gendistros.py）
.github/workflows/  test.yml（CI）/ release.yml（发布）
dist/               构建产物（build.sh 输出）
build.sh            交叉编译六平台（--install 装到 ~/.local/bin）
```

## 分层契约

```txt
cmd/barista            入口，只做 fang.Execute
internal/cli/       命令层：解析参数、组装 []Task；不碰业务逻辑
internal/workspace/ 工作区发现（向上找 .barista/）、repos.json / 两级 config.json 加载校验、上爬查找与检出根判定（upward.go）、.java-version 原子写
internal/gitrun/    git 域：exec 封装、单仓库操作、默认分支解析
internal/deps/      依赖图域：从 repos[].deps 声明建图；dangling 检测、SCC 环检测（Tarjan）、构建层级（1 + 依赖链最长路径，环成员共享层级）；纯函数，不碰文件系统与子进程
internal/jdk/       JDK 域：registry 读写（原子写）、probe（java -version / -XshowSettings 解析）、版本解析比较、.java-version 解析与 distro+major 解析（javaversion.go）、discover 候选收集（JAVA_HOME 系 env / sdkman / 平台安装位置 / PATH 反推）
internal/maven/     Maven 域：registry 读写（原子写）、probe（纯文件系统：bin/mvn 存在性 + lib/maven-core-*.jar 文件名解析版本，不起子进程）、版本比较（数字段 + qualifier token 比较，ComparableVersion 简化版）、模糊解析、wrapper properties 版本提取（wrapper.go）、discover 候选收集（MAVEN_HOME/M2_HOME / sdkman / brew / scoop / 平台位置 / PATH 反推）
internal/download/  通用下载（Range 断点续传、指数退避重试、4xx 不重试）、文件名解析（HEAD + Content-Disposition/最终 URL basename）与归档解压（剥首层、防 zip-slip/逃逸 symlink）
internal/upgrade/   自更新域：release manifest 拉取/解析/校验（base URL 可注入，默认 GitHub releases/latest/download）、版本比较（复用 maven.CompareVersions，dev/dirty 视为未知始终可升级）、sha256 校验、可执行文件自替换
internal/runner/    通用并发 worker pool（泛型，不绑定 git 语义）
internal/output/    Result 类型 + text / json / tui 三种 renderer
schemas/            JSON Schema 单一数据源（包即数据目录，同目录 go:embed）
```

核心契约：**cli 产出 `[]Task` → runner 产出 `[]Result` → renderer 只消费 Result**。renderer 不得触碰 git 逻辑。新增功能域（jdk、tool）时各自实现 Task，runner 和 output 不得为此修改。

repo 命令组（cli/repo/）管理工作区清单 repos.json，是唯一**写** repos.json 的入口（`init --scan` 的导入也复用同一 AddRepo 路径）：自带轻骨架（单仓库操作不走 runner/panel），`repo add <url>` 先注册后克隆——注册走 `workspace.AddRepo`（原子写，保留未知顶层字段与既有条目原始字节，baseUrl 前缀自动剥成相对存储；name 存在且 URL 等价 → 幂等跳过，URL 不同 → REPO_EXISTS exit 2），克隆复用 `gitrun.Clone`（目标已是 git 仓库先校验 origin 与 ResolvedURL 一致，不符报 REPO_REMOTE_MISMATCH exit 1）；clone 失败留下"已注册未检出"的合法状态，重跑自动续 clone。URL 等价比较统一走 `workspace.NormalizeURL`（去尾部 `/` 与 `.git`）。`repo list` 只读：按声明顺序列出清单条目，检出状态以 `<path>/.git` 存在性判定（不起子进程）；`repo remove` 经 `workspace.RemoveRepo` 注销条目（同一原子写契约），默认保留检出，`--delete` 仅当目标是 git 检出才删目录，确认契约同 uninstall（TTY 询问 / 非 TTY CONFIRMATION_REQUIRED / `--yes`）。

deps 命令组（cli/deps/）是**纯只读**的清单消费者：`repos[].deps` 声明（repo 名数组，人维护的稳定事实）经 `internal/deps` 建图，`show` 输出邻接表（dangling 引用与环如实标注），`order` 输出构建层级序——层级 = 1 + 依赖链最长路径，同层可并行构建，环成员折叠同层并标注；图在完整清单上构建，选择器（--repo/--label，语义同 git 组）只过滤输出。不克隆也可运行（不触碰检出）；无失败路径，环与 dangling 是事实展示而非失败（exit 0），仅用法/配置错误 exit 2。文本自排版（tabwriter，骨架镜像 repo 组），JSON 走 Envelope：results 按声明顺序，order 的层级放 detail.buildLevel。

`barista init [path]`（cli/init/）引导新工作区：`workspace.Init` 创建 `.barista/repos.json` skeleton（已存在永不覆盖，报 already initialized），目标路径不存在报 CONFIG_ERROR；`--scan` 经 `workspace.ScanCheckouts` 扫两层内（`*/.git`、`repos/*/.git`，跳过隐藏目录与 .barista）的检出，按 origin URL 逐条 AddRepo 导入（无 origin / 已注册 / name 冲突均跳过并记录原因），导入的 URL 同样走 baseUrl 转相对。

jdk 命令组的特殊性：registry 是 user 级文件，命令不依赖工作区（`use` 例外：jdk 组唯一需要工作区的命令，写 `<workspace>/.barista/properties.json` 的 `jdk` 键，且 cwd 上爬命中 `.git` 检出根时追加原子写 `<检出根>/.java-version`——内容为解析后精确版本，值不同覆盖并提示原值，写失败结果降级 failed 不回滚 properties，不受 `detect.files` 开关影响），故 cli/jdk 自带执行骨架（不调用 workspace.Load）；output 的 TextRenderer 绑定 git 语义（"repos" 汇总行、git 动作描述），jdk 域文本输出在 cli/jdk 内自行排版（tabwriter；which 例外，恒走 Envelope JSON，`--pathonly`/`path`/`home` 输出纯路径；env 默认输出 shell 导出语句供 eval 消费，`--shell sh|cmd|powershell`（别名 bash/zsh→sh、pwsh/ps→powershell），缺省自动检测当前 shell（Windows 用 x/sys/windows 读父进程映像名，其次 MSYSTEM/SHELL 标记；Unix 读 `$SHELL`），检测落空回退平台默认，`--json` 走 Envelope），色彩复用导出的 `output.Palette`（高亮语义仍集中在 color.go，命令实现不手写 ANSI），JSON 仍走 output.Envelope。discover 是 jdk 域唯一走 runner 并发的命令（每个候选路径一个 Task，probe 起子进程）；注册在主线程串行进行，命名按 `<distro><major>` 冲突追加 `-1`/`-2`。discover 幂等可重复：已注册路径（SamePath 比较）报 skipped，原因放 `Detail.reason`（不带 Error，重复跑不产生错误）；probe 失败的候选同样 skipped 但保留 Error 诊断。install 通过 Provider 接口解析发行版下载地址（temurin → Adoptium API；microsoft / corretto → permalink 直链，ArchiveURL 纯拼 URL；zulu / graalvm → `embeddedProvider` 读 go:embed 的 `internal/jdk/distros.json`，由 `scripts/gendistros.py` 定期刷新，URL 钉死无有效期、运行期零 API 调用），下载支持断点续传（Range 头）与指数退避重试（默认 4 次，4xx 不重试），TTY 下 stderr 渲染进度条（百分比/速度/ETA）；实现 `ChecksumProvider` 的 provider（graalvm 嵌入了 sha256 钉值）下载后先校验 sha256，不符报 JDK_CHECKSUM_MISMATCH；下载后解压到 `<installDir>/<name>`（剥归档首层目录），probe 校验 major 匹配才注册（`managed: true`），任何失败清理半成品目录。download 复用同一 Provider/ChecksumProvider 与下载骨架（进度条/重试/校验相同），但只下载不安装：不读注册表、不解压、不 probe；`--os`/`--arch` 缺省当前平台、可跨平台（非法值 exit 2），`--output` 缺省当前目录（值是已存在目录或以路径分隔符结尾按目录拼接文件名，否则视为完整文件路径）；文件名经 `download.ResolveFileName`（HEAD 跟随重定向，Content-Disposition 优先、其次最终 URL basename，失败静默回退合成名 `<distro>-jdk-<major>-<os>-<arch>`+从最终 URL 识别的扩展名，distro 名含 jdk 省略 `-jdk` 段）；目标已存在报 JDK_EXISTS；下载写 `<dest>.part` 成功后 rename，`.part` 留存重跑天然续传，sha256 不符删 `.part` 报 JDK_CHECKSUM_MISMATCH。available 经 `Provider.Available` 列出可安装项，数据源随 distro：temurin 实时查 API（available_releases 拿 major 列表与 LTS 标记，每 major 并发查 release_versions 取最新 GA 版本，单点失败只置空 version），permalink 两家返回静态 major 列表，zulu / graalvm 读嵌入数据离线返回；均按 major 升序并对照注册表标记 installed。uninstall 仅作用于 managed 条目，删除 `<installDir>/<name>` 整树并注销（级联清 defaults）；确认契约：TTY 交互询问，非 TTY 报 CONFIRMATION_REQUIRED（exit 2），`--yes` 直通。

maven 命令组镜像 jdk 的骨架与契约（自带执行骨架、tabwriter 文本、Envelope JSON、install/uninstall 同样的半成品清理与确认契约），差异点：registry 为 `~/.barista/maven.json`（`installations` + `default` + `jdk`）；probe 是纯文件系统探测（校验 `bin/mvn`/`bin/mvn.cmd` 存在、从 `lib/maven-core-*.jar` 文件名解析版本），不起子进程，因此 discover 串行不走 runner，也不依赖 java 可用。自动命名为 `maven-<major.minor>`（版本号保留两段，qualifier 不进名字），add/discover/install 共用，冲突走 AvailableName 追加 `-1`/`-2`；`add <path> [--name]`（--name 禁止形似版本号，避免与模糊解析歧义）。解析（which/path/home，三命令独立可见）：精确名字 → 精确版本 → 数字段前缀逐级扩大（`3.9.11`→`3.9.*`→`3.*`）取 CompareVersions 最大者 → MAVEN_NOT_FOUND；模糊解析只用于只读解析，remove/uninstall/set-default 强制精确名字。install 单 Provider（Apache archive：`archive.apache.org/dist/maven/maven-<major>/<version>/`），一律 tar.gz，下载后强制 sha512 校验（`.sha512` 旁挂文件，不匹配报 MAVEN_CHECKSUM_MISMATCH）。`maven config` 内省命令打印合并后的有效配置及来源（source 枚举：workspace/user/repo/builtin/ambient/none），含 cwd 命中的 repo、settings 与 repo.local 注入状态。

`barista mvn` 是**执行器**而非管理命令（cli/mvn/，不走 runner/Result 契约）：`barista mvn [flags] -- <mvn args>`，`--` 后的参数原样透传（缺 `--` 直接给 goal 报 USAGE_ERROR exit 2）。环境组装是纯函数 `planExec`（plan.go，同包单测）：maven 安装按 `wrapper distributionUrl 版本 > workspace maven.default property > maven.json default` 解析；JDK 按 `--jdk > .java-version 文件 > cwd 命中 repo 的 properties["jdk"] > workspace jdk > maven.json jdk > 环境原样`，显式指定的任一级（含文件声明）解析失败响亮报 JDK_NOT_FOUND/MAVEN_NOT_FOUND（exit 2，message 带来源），命中则只对子进程设 JAVA_HOME。文件检测（detectJavaVersionFile / wrapperVersion，wrapper 复用 launch.go 的 findBasedir 定位 `.mvn/wrapper/maven-wrapper.properties`）在 buildPlan 装配层完成并以 planInput 字段注入，planExec 保持纯函数；查找边界为最近 `.git` 检出根（workspace.FindGitRoot，不依赖 repos.json 注册），workspace properties 键 `detect.files=false` 整体关闭。注入规则：`.barista/maven/settings.xml` 存在即注入 `-s`（同目录 `settings-security.xml` 注入 `-Dsettings.security`）；workspace property `maven.repo.local` 注入 `-Dmaven.repo.local`（相对路径锚定 workspace root）；**透传参数已含 `-s`/`--settings`/对应 `-D` 时跳过该注入**（用户显式优先）。执行：Unix 直接 `bin/mvn`，Windows 经 `cmd /c bin/mvn.cmd`；stdin/stdout/stderr 直接挂父进程（流式透传，无 TUI/渲染层）；mvn 非零退出 → exit 1，barista 自身配置错误 → exit 2 + 机器可读码；`--json` 只影响 exec 前的错误（ErrorEnvelope）与 --dry-run 输出，exec 后输出归属 mvn。workspace 发现是机会主义的（无 workspace = 跳过 repo/workspace 两级覆盖，纯 user 级默认透传）。

`barista java` 是同型执行器（cli/java/，镜像 cli/mvn 骨架）：`barista java [--jdk spec] [--dry-run] -- <java args>`。JDK 解析链 `--jdk > .java-version 文件 > repo properties["jdk"] > workspace jdk > ambient（PATH 查找 java）`，无 maven.json jdk 一级；文件检测与边界语义同 cli/mvn（detectJavaVersionFile，detect.files 开关共用）；显式级别失败响亮报 JDK_NOT_FOUND（exit 2），ambient 也落空同样 JDK_NOT_FOUND。命中注册 JDK 时直接 exec 其 `bin/java`（Windows `bin/java.exe`，不经 cmd 包装），只对子进程设 JAVA_HOME；java 非零退出 → exit 1，启动失败 → JAVA_EXEC_FAILED（exit 2）。环境组装同为纯函数 planExec（ambient 的 PATH 查找在 buildPlan 完成、以 ambientBin 注入，保持 planExec 可测）。

`barista doctor` 是体检命令（cli/doctor/）：只诊断不修复，每项检查一个 Result（ok / skipped 信息项带 reason / failed 带修复 hint），浅查项装配期同步执行，probe 与 repo 检查走 runner 并发；始终查 user 级（git on PATH、JAVA_HOME 有效性、config/jdk/maven registry 可解析性与引用完整性——defaults/default/jdk/installDir，Maven 条目版本用纯文件系统 probe 即时比对），机会主义加查 workspace 级（.barista 可加载、repo 检出存在、properties 与 per-repo properties 的 jdk/maven.default/maven.launch 可解析、settings.xml 存在性、逐 repo 浅查 .java-version 与 maven-wrapper.properties 的可解析性——wrapper 检查只看检出根不上爬）；`--deep` 追加 JDK 重 probe（起 java 子进程比对注册版本）与 repo origin 对清单 URL 的归一化比对。文本按 scope 分组自排版（不复用绑定 git 语义的 TextRenderer），JSON 走 Envelope（detail 含 scope/check）。exit 0 全过 / 1 有 failed / 2 用法错误。

`barista upgrade` 是自更新命令（cli/upgrade/ + internal/upgrade/），与 `jdk available` 是仅有的两个主动联网命令（均为用户显式调用才联网）：读最新 release 的 `manifest.json` asset（固定 URL，无 API 调用），比较版本（当前 ≥ 最新幂等报 ok；dev/dirty 构建视为未知始终可升级），下载平台 asset（复用 download 包：断点续传 + 重试 + TTY 进度条），sha256 校验不符报 UPGRADE_CHECKSUM_MISMATCH，然后替换 `os.Executable()`——Unix 临时文件 rename 原子覆盖；Windows 运行中 exe 不可覆盖但可改名，先 rename 为 `.old` 再写入，`.old` 下次启动清理。确认契约同 uninstall：TTY 询问 / 非 TTY CONFIRMATION_REQUIRED（exit 2）/ `--yes` 直通；`--check` 只检测不下载。清单由 release workflow 调用 `python3 scripts/genmanifest.py` 生成（每个 asset 的 file/sha256/size 由该脚本计算；workflow 另产出独立的 checksums.txt，不被 manifest 消费），发布前经 `barista schema validate manifest` 校验（schema 在 schemas/manifest.schema.json）。

启动模式（launch.go）：`--launch` > repo `properties["maven.launch"]` > workspace `properties["maven.launch"]` > 默认 `script`；非法值报 CONFIG_ERROR。`script` 即上述包装脚本路径。`java` 绕过包装脚本直启 java（消除 `cmd /c` 二次解析对 `%`/`&`/`|` 参数的变形风险，双平台同一代码路径），复刻 mvn 脚本契约（3.6–3.9 实测同构）：java 取 JDK 路径的 `bin/java`（ambient 时 PATH 查找）、glob `boot/plexus-classworlds-*.jar` 恰一个 + `bin/m2.conf` 存在（否则 MAVEN_EXEC_FAILED，hint 回退 `--launch script`）、basedir 上爬 `.mvn`（感知 `-f`/`--file`，`MAVEN_BASEDIR` env 优先，落空回 cwd）、读 `.mvn/jvm.config`、透传 `MAVEN_OPTS`/`MAVEN_DEBUG_OPTS`、`MAVEN_ARGS` 仅 ≥3.9 追加、`-Dlibrary.jansi.path` 在 `lib/jansi-native` 存在时设置。差异声明：java 模式不执行 `mavenrc_pre/post` 钩子。

## 输出契约（面向 AI Agent 设计）

- stdout/stderr 严格分离：结果（含 JSON）走 stdout，诊断走 stderr
- JSON 输出带 `schemaVersion: 1`；`results` 按 repos.json 声明顺序（确定性，不随并发漂移）
- 错误用机器可读 code，枚举定义在 `internal/output/result.go`，新增场景应加新码而不是复用近似码
- exit code：0 全成功 / 1 任一检查/操作 failed（jdk/maven 解析失败、upgrade 网络/校验/替换失败、schema 校验不通过、mvn/java 子进程非零等） / 2 用法、配置、工作区错误
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
- config.json 顶层字段只剩 `parallel`/`color`（properties 已拆出为独立文件）
- `<workspace>/.barista/properties.json`：workspace 级偏好 KV map（点分键，Java properties 风格；值允许 string/number/boolean 标量），**仅 workspace 级存在**，user 级无此文件；缺失视为空。当前被读取的键：`jdk`（跨域公用，由 `barista jdk use` 写入）、`maven.default`（由 `barista maven set-default --scope workspace` 写入）、`maven.repo.local`、`maven.launch`、`detect.files`（`false` 关闭 mvn/java 的 `.java-version` 与 wrapper 文件检测，手改写入）、`git.fetch.prune`/`git.pull.rebase`（由 `barista git fetch`/`git pull` 在未显式传 flag 时读取，手改写入）；未知键宽容忽略，CLI 写入走 `workspace.SetProperty`（通用 map 读写 + 临时文件 rename 原子写，未知键全保留）
- maven 域偏好分层：user 级唯一来源是 maven.json 的 `default`/`jdk` 字段（`jdk` 由 `barista maven set-jdk` 写入）；workspace 级覆盖放 `<workspace>/.barista/properties.json`——`jdk` 是跨域公用属性键（工具链声明，不限 maven），由 `barista jdk use` 写入，`maven.default` 仍由 `barista maven set-default --scope workspace` 写入。解析链：`--flag > workspace properties > maven.json > 缺省（default 报 MAVEN_NOT_FOUND；jdk 回退环境原样）`。workspace 覆盖解析不到目标时响亮报错，不静默回退
- `maven.repo.local`：workspace properties.json 键，`barista mvn` 显式设置时注入 `-Dmaven.repo.local`（相对路径锚定 workspace root）；CLI 注入优先级高于 settings.xml 的 `<localRepository>`，两者同设时 property 赢
- `<workspace>/.barista/maven/`：maven 执行约定目录——`settings.xml` 存在即被 `barista mvn` 以 `-s` 注入（零配置私有 settings），`settings-security.xml` 同理注入 `-Dsettings.security`
- maven 命令的 workspace 发现是机会主义的（FindRoot 找不到不算错误，仅意味着无 workspace 级覆盖）；**home 目录守卫**：向上找到的 `.barista` 若就是 user 级 `~/.barista`（root == 用户主目录），不视为 workspace——防止把 workspace 偏好写进 user 级文件。守卫实现收敛在 `workspace.FindWorkspaceRoot`，maven 组与 mvn 命令共用
- workspace level：`<workspace>/.barista/config.json`（与 user level 同 schema，覆盖 user level）
- `<workspace>/.barista/repos.json`：仓库清单（纯事实：baseUrl、defaultBranch、repos）；repo 条目的 `deps` 声明本 repo 构建前需先构建的清单内 repo 名（稳定事实，人维护，供 `barista deps` 消费）；repo 条目另有 `properties` map（string→string），承载 **per-repo 覆盖**——当前被读取的键：`jdk`（工具链声明，不限 maven；cwd 命中 repo 时优先于 workspace jdk）、`maven.launch`（java|script，同理）；未知键宽容忽略，其他域可复用同一容器
- 优先级（低→高）：内置默认 → user level → workspace level（config.json + properties.json，含 git.* 键）→ repo properties（per-repo 值）→ 命令行 flag；bool flag 用 `cmd.Flags().Changed()` 判断是否显式设置
- 归属判定规则：**客观事实**（baseUrl、defaultBranch、repos）放 repos.json 顶层字段；**workspace 级行为/环境偏好**（`git.fetch.prune`、`jdk`、`maven.*` 等）放 properties.json。拿不准时按此规则裁决。键命名约定：跨域工具链声明用裸名（`jdk`），域专属偏好用点分前缀（`maven.*`、`git.*`）
- 所有层级对未知字段宽容（忽略）

## 行为规则

- 一切操作幂等：重复执行不产生额外副作用；已满足终态的仓库报 `skipped` 并带原因码
- 默认分支解析链（`internal/gitrun/branch.go`）：repo → top → origin-head → probe(main/master/trunk)，**逐级回退**——候选分支在该仓库无真实 ref 时落到下一级，而不是失败；全落空才报 `DEFAULT_BRANCH_UNRESOLVED`
- checkout 新建分支必须 `--no-track`（否则 push.default=simple 推不上去）；push 在无 upstream 时自动 `-u origin <branch>`
- status 不静默联网，ahead/behind 基于本地缓存的远端引用
- shell 补全（cli/comp，cobra ValidArgsFunction / RegisterFlagCompletionFunc）只读本地 registry / repos.json，任何读取失败静默返回空候选，永不起子进程、不联网
