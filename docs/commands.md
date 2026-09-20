# 命令参考

barista CLI 的完整命令参考。设计契约（分层、输出、退出码）见 [architecture.md](architecture.md)。

## 全局约定

### 全局 flags

| flag         | 默认 | 说明                                 |
| ------------ | ---- | ------------------------------------ |
| `--json`     | off  | 结果以 JSON envelope 输出到 stdout   |
| `--parallel` | 10   | 并发执行的仓库/任务数上限            |

`--parallel` 未显式给出时回退到 config 的 `parallel` 字段（git 命令组读 user+workspace 合并后的 config；`jdk discover` 只读 user 级 config）。

fang 框架另自带隐藏的 `man` 命令（生成 manpages）与 root 的 `--version` flag。

### 输出形态

- text：非 TTY 逐条输出；TTY 下 git 命令组渲染实时面板
- JSON：`--json` 时输出 envelope，含 `schemaVersion: 1`、`command`、`success`、`results`；结果顺序与声明顺序一致
- 错误：stderr 输出 `barista: CODE: message`（可带 `hint:` 行），`--json` 时 stdout 另出 error envelope
- 例外：`jdk`/`maven` 的 `which`（非 `--pathonly`）恒输出 JSON envelope，不受 `--json` 影响；`schema` 组只有 `show`（schema 原文）与 `validate`（校验结果）恒输出 JSON（`list` 与裸 `barista schema` 输出纯文本列表），其错误格式为 `barista: <msg>`（无 CODE、无 hint）；jdk/maven 的 `path`/`home`/`which --pathonly` 输出纯路径，无尾随换行
- jdk/maven 单目标命令（add/remove/set-default/set-jdk/use/install/uninstall）：text 模式下结果级失败以 `failed <name>: CODE: message`（可带 `hint:` 行）输出到 stderr；`--json` 时失败只体现在 envelope 的 results 中

### 退出码

| 码  | 含义                                               |
| --- | -------------------------------------------------- |
| 0   | 全部成功                                           |
| 1   | 至少一个结果失败（含 schema 校验不通过、mvn 子进程非零退出、which 解析失败） |
| 2   | 用法错误、配置错误、确认类错误（如 CONFIRMATION_REQUIRED） |

通用红线：stdout/stderr 严格分离；非 TTY 永不阻塞询问；一切操作幂等，可重复执行。

### shell 补全

内置 `completion` 命令生成各 shell 的补全脚本：

```bash
# bash（Linux）
barista completion bash > ~/.local/share/bash-completion/completions/barista
# zsh（补全目录在 fpath 中）
barista completion zsh > ~/.zsh/completions/_barista
# fish
barista completion fish > ~/.config/fish/completions/barista.fish
# PowerShell（加进 $PROFILE）
barista completion powershell >> $PROFILE
```

除命令名/flag 名的静态补全外，以下位置有动态补全（只读本地 registry / repos.json，失败静默降级为空，永不起子进程联网）：

- `--repo` / `--label`（git 组）：当前工作区的仓库名与标签
- jdk 组位置参数：`which`/`path`/`home`/`env`/`use` 补 name + major；`remove`/`uninstall` 补 name；`set-default` 第一段补 major、第二段补 name
- maven 组位置参数：`which` 补 name + version；`remove`/`uninstall`/`set-default` 补 name；`set-jdk` 补 JDK spec
- `mvn --jdk` / `java --jdk`：JDK spec
- `jdk env --shell`：sh/cmd/powershell/pwsh/ps（bash/zsh 是输入别名，不提供为补全候选）
- `schema show` / `validate` 第一段：schema 名；`init [path]` 与 `repo add --path`：目录

## init — 工作区引导

在目标目录（缺省当前目录）创建 `.barista/repos.json` skeleton，可选导入已存在的 git 检出。

```txt
barista init [path] [--repo-base-url <url>] [--default-branch <name>] [--scan]
```

| flag               | 说明                                                          |
| ------------------ | ------------------------------------------------------------- |
| `--repo-base-url`       | 清单的相对 URL 前缀                                            |
| `--default-branch` | 所有仓库的默认分支兜底                                         |
| `--scan`           | 扫描两层内的 git 检出（`*/.git`、`repos/*/.git`），按 origin URL 预填 repos |

### 要点

- 幂等：`repos.json` 已存在永不覆盖，报 already initialized；带 `--scan` 重跑则只导入缺失条目（已注册的跳过）
- 导入时 name 取目录名，URL 以 `baseUrl` 为前缀自动转相对；无 origin 的检出跳过；path 不存在的目标报 `CONFIG_ERROR`（exit 2）
- exit 0 成功或已初始化 / 2 用法与配置错误

```bash
barista init --repo-base-url git@github.com:org/ --scan
```

## git — 多仓库批量操作

要求在 `.barista/` 工作区内运行（向上查找）。仓库清单来自 `.barista/repos.json`。

### 选择器

| flag      | 说明                                     |
| --------- | ---------------------------------------- |
| `--label` | 按 label 选仓库，可重复，取并集          |
| `--repo`  | 按 name 选仓库，可重复，与 label 取并集  |

不给选择器时作用于全部仓库；无匹配报 `NO_MATCHING_REPOS`（exit 2）。

### 子命令

| 命令                | 行为                                       | 特有 flag  |
| ------------------- | ------------------------------------------ | ---------- |
| `status`            | 离线查看各仓库状态，末尾汇总有变更的仓库   | —          |
| `clone`             | 克隆缺失的仓库（已存在则 skipped `REPO_EXISTS`） | —          |
| `fetch`             | 批量 fetch                                 | `--prune`  |
| `pull`              | 批量 pull                                  | `--rebase` |
| `push`              | 批量 push                                  | `--tags`   |
| `checkout <branch>` | 切换或创建分支                             | —          |

### 要点

- `--prune` / `--rebase` 未显式给出时回退 properties.json 的 `git.fetch.prune` / `git.pull.rebase`
- checkout 三分支：本地分支已存在 → 切换；`origin/<branch>` 存在 → `checkout --track`；都没有 → 从默认分支创建（`--no-track`）
- checkout 遇未提交变更（staged/modified）报 skipped `DIRTY_WORKTREE`
- text 输出中 repo 名按本批最长名动态对齐（上限 40 字符，超长截断为 `…`）；分支按类别着色：`main`/`master`/`trunk` 绿、`develop`/`dev` 蓝、`release/**` 青、`feature/**` 黄、`hotfix/**`/`fix/**`/`bugfix/**` 品红，其余不着色（`origin/` 前缀不影响分类）
- status 中未跟踪 upstream 的本地分支带 `*` 后缀，且 ahead/behind 显示 `-`（无 upstream 可比较）；JSON detail 含 `tracked` 布尔字段

```bash
barista git status --label java
barista git checkout feature/x --repo order-service --repo user-service
```

## repo — 工作区仓库清单管理

管理 `<workspace>/.barista/repos.json`，需要在工作区内运行。

| 命令              | 行为                                          |
| ----------------- | --------------------------------------------- |
| `add <url>`       | 注册新仓库到 repos.json 并克隆（一步完成）    |

### add flags

| flag         | 说明                                                        |
| ------------ | ----------------------------------------------------------- |
| `--name`     | 清单中的仓库名（缺省从 URL basename 推导，去 `.git`）       |
| `--path`     | 检出路径（相对 workspace 根，缺省 `repos/<name>`）          |
| `--label`    | 打标签供 `--label` 过滤（可重复）                           |
| `--no-clone` | 只注册不克隆，之后用 `barista git clone` 补                 |

### 要点

- **先注册后克隆**：clone 失败时条目已在清单中（合法状态），重跑 `repo add` 自动跳过注册、只重试克隆——全程幂等
- name 推导：取 URL 最后一段（`/` 或 `:` 分隔），去尾部 `/` 与 `.git`；推导不出时报 `USAGE_ERROR` 并提示 `--name`
- URL 存储：以 `baseUrl` 为前缀的绝对 URL 自动剥前缀存为相对形式；传相对 URL 但清单无 `baseUrl` 报 `CONFIG_ERROR`；本地绝对路径（`/srv/...`、`C:/...`、`\\server\share\...`）视为绝对原样存储与克隆（注意：绝对路径不可跨平台移植，团队共享清单建议用 `file://` 或远端 URL）
- 幂等与冲突：name 已存在且 URL 等价（忽略尾部 `/`、`.git` 差异）→ 跳过注册继续克隆；name 存在但 URL 不同 → `REPO_EXISTS`（exit 2）
- 目标路径已是 git 仓库时校验其 origin 与清单解析 URL 一致，不符报 `REPO_REMOTE_MISMATCH`（exit 1），不静默跳过
- 写 repos.json 为原子写，保留手改的未知顶层字段与既有条目的未知字段；新条目追加到 `repos` 数组末尾
- exit 0 成功 / 1 克隆或 origin 校验失败（此时已注册）/ 2 用法与配置错误

```bash
barista repo add git@github.com:org/order-service.git --label java
barista repo add order-service --no-clone    # 相对 URL，走 baseUrl
```

## jdk — JDK 注册表管理

注册表为 user 级 `~/.barista/jdk.json`。托管安装目录默认 `~/.barista/toolchains/jdk/`（jdk.json 顶层 `installDir` 字段，用 `barista jdk set-install-dir` 设置）。`use` 是 jdk 组唯一需要工作区的命令。

### 子命令

| 命令                         | 行为                                                        |
| ---------------------------- | ----------------------------------------------------------- |
| `discover`                   | 扫描 JAVA_HOME 系 env / sdkman / 平台安装位置 / PATH 并注册 |
| `add <name> <path>`          | 注册已安装的 JDK（起 `java` 探测版本与发行版；`javac` 仅做存在性检查） |
| `install <distro><major>`    | 下载并安装到托管目录（支持 temurin / microsoft / corretto / zulu / graalvm） |
| `available`                  | 列出远端可安装的 JDK（major / 最新版本 / LTS / 是否已装）   |
| `list`                       | 列出已注册 JDK（别名 `ls`）                                 |
| `which <major\|name>`        | 解析 JDK 并打印信息（恒输出 JSON）                          |
| `path` / `home <major\|name>` | 只打印路径（`which --pathonly` 的快捷方式）                 |
| `env <major\|name>`           | 打印 JAVA_HOME/PATH 导出语句（`--shell sh\|cmd\|powershell`，缺省自动检测当前 shell，供 eval / CI 消费） |
| `set-default <major> <name>` | 设置某个 major 版本的默认 JDK                               |
| `set-install-dir <path>`     | 设置托管安装根目录（写入 jdk.json `installDir`；`--reset` 恢复内置默认） |
| `use <major\|name>`          | 设置当前工作区的 JDK（写 workspace `.barista/properties.json` 的 `jdk` 键） |
| `remove <name>`              | 仅注销，保留磁盘文件（级联清理指向它的 defaults）           |
| `uninstall <name>`           | 删除 managed 安装并注销（级联清理 defaults）                |

### 要点

- discover：幂等可重复；已注册路径报 skipped；是唯一走 `--parallel` 并发的 jdk 命令（每个候选路径一个探测任务）
- add：名称必须匹配 `[a-z0-9][a-z0-9._-]*` 且不能是纯数字（避免与 major 版本解析歧义），不符报 USAGE_ERROR（exit 2）；`--default` 同时设为该 major 的默认
- install：命名即 `<distro><major>`；下载支持断点续传与指数退避重试，TTY 下 stderr 渲染进度条；嵌入数据的 distro（zulu / graalvm）带 sha256 钉值，下载后校验，不符报 `JDK_CHECKSUM_MISMATCH`；解压后 probe 校验 major 匹配才注册为 `managed: true`，任何失败清理半成品目录
- available：列出各 distro 可安装项；数据源按 distro 而异——temurin 实时查 Adoptium API（用户显式调用才联网），microsoft / corretto 为静态 major 列表（version 留空，permalink 始终指向最新 GA），zulu / graalvm 读嵌入二进制的 distros.json（离线，由 `scripts/gendistros.py` 定期刷新）；按 distro + major 升序输出，tags 标记 `lts` / `latest`（最新 LTS）/ `installed`（对照注册表）/ `unsupported-platform`；网络失败报 `JDK_AVAILABLE_FAILED`（exit 1）
- which 解析：传 major 先取该 major 的 default，否则取最新；传 name 精确匹配；非 `--pathonly` 时恒输出 JSON envelope（不受 `--json` 影响），解析失败 exit 1
- env：与 which/path 同款解析；默认输出 JAVA_HOME 与 PATH（前置 `<jdk>/bin`）导出语句；`--shell` 支持别名（bash/zsh→sh，pwsh/ps→powershell），缺省自动检测当前 shell（Windows 按父进程名，其次 MSYSTEM/SHELL 环境标记；Unix 读 `$SHELL`），检测不到回退平台默认（Windows → powershell，其余 → sh）；`--json` 时改输出 envelope（含 javaHome/bin/shell），解析失败 exit 1
- remove：直接注销，无确认、不删文件（幂等），并级联清理 jdk.json 中所有指向该 JDK 的 defaults
- uninstall：仅作用于 managed 条目；确认契约为 TTY 交互询问，非 TTY 报 `CONFIRMATION_REQUIRED`（exit 2），`--yes` 直通

```bash
barista jdk discover
barista jdk install temurin17
barista jdk add my-jdk8 /opt/jdk8 --default
barista jdk which 17
```

## maven — Maven 注册表管理

注册表为 user 级 `~/.barista/maven.json`。托管安装目录默认 `~/.barista/toolchains/maven/`（maven.json 顶层 `installDir` 字段，用 `barista maven set-install-dir` 设置）。`set-default` 支持 `--scope user|workspace`（默认 user；workspace 写入 `<workspace>/.barista/properties.json`）；`set-jdk` 只写 user 级（workspace 级 JDK 用 `barista jdk use`）。

### 子命令

| 命令                    | 行为                                                                   |
| ----------------------- | ---------------------------------------------------------------------- |
| `discover`              | 扫描 MAVEN_HOME/M2_HOME / sdkman / brew / scoop / 平台位置 / PATH 并注册 |
| `add <path>`            | 注册已安装的 Maven（从 `lib/maven-core-*.jar` 文件名读版本，不起子进程） |
| `install <version>`     | 从 Apache archive 下载（sha512 校验）安装到托管目录                     |
| `list`                  | 列出已注册 Maven（别名 `ls`）                                            |
| `which [name\|version]` | 按名或版本解析（版本逐级放宽）；不传参数取生效默认（恒输出 JSON）      |
| `path` / `home`         | 只打印 maven home 路径（`which --pathonly` 的快捷方式）                |
| `set-default <name>`    | 设置默认 Maven（`--scope` 选择写入层级）                               |
| `set-jdk <major\|name>` | 设置运行 Maven 的 JDK（按 jdk 注册表解析，写 user maven.json）         |
| `set-install-dir <path>` | 设置托管安装根目录（写入 maven.json `installDir`；`--reset` 恢复内置默认） |
| `config`                | 查看生效配置（default / jdk / installDir 及 workspace 级项）及其来源   |
| `remove <name>`         | 仅注销，保留磁盘文件（若为 user 级 default 一并清除）                  |
| `uninstall <name>`      | 删除 managed 安装并注销（同样清除指向它的 user 级 default）            |

### 要点

- add：`--name` 指定注册名（默认 `maven-<major.minor>`，冲突自动追加序号）；名称必须匹配 `[a-z0-9][a-z0-9._-]*` 且不能形如版本号（如 `3.9` / `3.9.9`，避免与版本解析歧义），不符按失败结果报 CONFIG_ERROR；`--default` 同时设为默认
- set-default：对 name 校验同一名称模式 `[a-z0-9][a-z0-9._-]*`（不符为用法错误，exit 2），但不查版本号形态（remove/uninstall/set-default 本就强制精确名，不走模糊解析）
- install：解压后 probe 校验版本与请求完全一致，失败清理目录
- which 版本解析逐级放宽：`3.9.9` → `3.9` → `3`；非 `--pathonly` 时恒输出 JSON envelope，解析失败 exit 1
- 生效优先级：workspace 配置 `maven.default` / `jdk` 覆盖 user 注册表字段
- set-jdk：只写 user 级 maven.json，写入的是用户给的原始 spec（如 `17`），而非解析后的注册名；workspace 级覆盖用 `barista jdk use`
- remove/uninstall 契约与 jdk 组相同：remove 直接注销无确认；uninstall 仅 managed，TTY 询问 / 非 TTY exit 2 / `--yes` 直通；两者若为 user 级 default 均一并清除该 default

```bash
barista maven install 3.9.9
barista maven set-default maven-3.9
barista jdk use 17
barista maven config
```

## mvn — 工作区感知的 Maven 执行器

在当前目录运行 Maven，`--` 之后的参数原样透传。

```txt
barista mvn [flags] -- <mvn args...>
```

### flags

| flag        | 说明                                                          |
| ----------- | ------------------------------------------------------------- |
| `--jdk`     | JDK spec（注册名或 major 版本），覆盖所有配置层级             |
| `--launch`  | 启动方式：`script`（默认，mvn/mvn.cmd wrapper）或 `java`（直接 java 启动 classworlds jar，绕开 wrapper 的引号陷阱，但也跳过 mavenrc 钩子） |
| `--dry-run` | 打印解析出的环境与完整命令行，不执行                          |

### 解析链（高 → 低）

- Maven 安装：workspace properties.json `maven.default` > user maven.json default（无 repo 级）
- JDK：`--jdk` > repo `properties["jdk"]` > workspace properties.json `jdk` > user maven.json jdk > ambient（不动 JAVA_HOME/PATH）
- settings.xml：存在 `.barista/maven/settings.xml` 时注入 `-s`（自行传 `-s`/`--settings` 则跳过）；同目录 `settings-security.xml` 存在时配套注入 `-Dsettings.security`（自行传 `-s`/`--settings` 或 `-Dsettings.security` 则跳过）
- 本地仓库：workspace properties.json `maven.repo.local` 注入 `-Dmaven.repo.local`（自行传则跳过）；相对路径基于 workspace 根，支持 `~` 展开
- launch：`--launch` > repo `maven.launch` > workspace properties.json `maven.launch` > `script`

### 其他行为

- 解析到 JDK 时子进程 JAVA_HOME 被替换为该 JDK
- Maven 子进程非零退出时 barista exit 1；stdout/stderr 直接继承
- `--dry-run` 的 `--json` 输出含 maven / jdk / launch / args / command 等解析明细

```bash
barista mvn -- clean install -DskipTests
barista mvn --jdk 17 --dry-run -- -q validate
```

## java — 工作区感知的 java 执行器

在当前目录用解析出的 JDK 直接运行 java，`--` 之后的参数原样透传。

```txt
barista java [flags] -- <java args...>
```

### flags

| flag        | 说明                                                      |
| ----------- | --------------------------------------------------------- |
| `--jdk`     | JDK spec（注册名或 major 版本），覆盖所有配置层级         |
| `--dry-run` | 打印解析出的 JDK 与完整命令行，不执行                     |

### 解析链（高 → 低）

- JDK：`--jdk` > repo `properties["jdk"]` > workspace properties.json `jdk` > ambient（PATH 中的 java，不动 JAVA_HOME/PATH）
- 显式层级（flag/repo/workspace）解析不到注册 JDK 时响亮报 `JDK_NOT_FOUND`（exit 2，message 带来源），不静默回退
- 无任何声明且 PATH 无 java 时报 `JDK_NOT_FOUND`（exit 2）

### 其他行为

- 解析到注册 JDK 时直接执行其 `bin/java`（Windows 为 `bin/java.exe`，不经 cmd 包装），子进程 JAVA_HOME 被替换为该 JDK
- java 子进程非零退出时 barista exit 1；stdout/stderr 直接继承
- 缺 `--` 直接给参数报 `USAGE_ERROR`（exit 2）；`--dry-run` 的 `--json` 输出含 jdk / bin / args / command 明细

```bash
barista java -- -version
barista java --jdk 17 --dry-run -- -jar app.jar
```

## doctor — 环境体检

诊断 user 级环境，在 workspace 内时自动加查 workspace 级（不在 workspace 不算错误）。只诊断不修复，每个 failed 检查带修复 hint。

```txt
barista doctor [--deep]
```

| flag     | 说明                                                              |
| -------- | ----------------------------------------------------------------- |
| `--deep` | 追加起子进程的检查：重新 probe 每个 JDK（`java -version` 比对注册版本）、逐 repo 校验 origin 与清单 URL 一致 |

### 检查项

- user 级：git 在 PATH；`JAVA_HOME` 有效性（未设置是合法的 ambient 状态，报 skipped）；`config.json` / `jdk.json` / `maven.json` 可解析；每个 JDK 条目 `bin/java` 存在、每个 Maven 安装 probe 版本与注册一致（纯文件系统）；`defaults` / `default` / `jdk` / `installDir` 引用可解析
- workspace 级：`.barista` 整体可加载（repos.json / config.json / properties.json）；每个 repo 检出存在（未克隆报 skipped，含补救命令）；workspace properties 的 `jdk` / `maven.default` / `maven.launch` 与 per-repo properties 的 `jdk` / `maven.launch`（无 per-repo `maven.default`，与 mvn 解析链"无 repo 级"一致）可解析或合法；`settings.xml` / `settings-security.xml` 存在性（不存在报 skipped，可选文件）

### 要点

- 检查并发执行（`--parallel` 生效），结果按声明顺序输出
- 结果三态：ok / skipped（不适用或信息项，reason 在 detail）/ failed（带 hint）；exit 0 全过 / 1 有 failed / 2 用法错误
- `--json` 每项 detail 含 `scope`（user|workspace）与 `check`（检查 id，如 `jdkInstall` / `repoCheckout`）

## upgrade — 自更新

从最新 GitHub release 的清单文件（`manifest.json` asset）检测并应用自更新。主动联网的命令只有 `upgrade` 与 `jdk available`（均为用户显式调用才联网），其他命令永不被动检测更新。

```txt
barista upgrade [--check] [--yes]
```

| flag      | 说明                                       |
| --------- | ------------------------------------------ |
| `--check` | 只检测是否有新版本，不下载不替换           |
| `--yes`   | 跳过确认提示（非 TTY 下必需）              |

### 行为

- 读取 `releases/latest/download/manifest.json`（无 API 调用、无鉴权）；当前版本 ≥ 最新时报 already up to date（幂等）；dev/dirty 构建视为未知版本，始终可升级到最新 release
- 下载对应 GOOS/GOARCH 的 asset（断点续传 + 指数退避重试，TTY 下 stderr 渲染进度条），完成后 sha256 校验，不符报 `UPGRADE_CHECKSUM_MISMATCH`
- 替换当前可执行文件：Unix 临时文件 + rename 原子覆盖；Windows 先把运行中的旧 exe 改名为 `.old` 再写入新文件（`.old` 下次运行自动清理）；目标不可写报 `UPGRADE_REPLACE_FAILED` 并带 hint
- 确认契约：TTY 交互询问，非 TTY 报 `CONFIRMATION_REQUIRED`（exit 2），`--yes` 直通
- exit 0 已最新、升级成功或 TTY 下回答 no 取消（结果标记 skipped/aborted） / 1 网络、校验或替换失败 / 2 确认缺失等用法错误

清单文件由 release workflow 生成（六平台 asset 的 file/sha256/size），发布前用 `barista schema validate manifest <file>` 校验（不带 file 会解析到工作区默认路径 `<workspace>/.barista/manifest.json`）。

## schema — 内置 JSON Schema 工具

供 AI Agent 与编辑器在线发现、校验配置。schema 单一数据源在 `schemas/` 包。

| 命令                          | 行为                                         |
| ----------------------------- | -------------------------------------------- |
| `list`                        | 列出可用 schema（repos / config / jdk / maven / properties / manifest） |
| `show <name>`                 | 输出 schema 原文 JSON                        |
| `validate <name> [file]`      | 校验配置文件                                 |

### validate 默认文件

- 显式给 `file` 时校验该文件（此时不可再用 `--scope`）
- `jdk` / `maven`：默认校验对应 user 级注册表（`~/.barista/jdk.json` / `maven.json`）
- `config`：默认同时校验 user 与 workspace 两级；`--scope user|workspace` 限定单级
- `properties`：默认校验当前工作区 `.barista/properties.json`
- 其他（如 `repos`）：默认校验当前工作区 `.barista/<name>.json`

### 退出码与输出

- 校验不通过：exit 1，结果 JSON 的 `errors` 列出问题
- 用法/环境错误（未知 schema、文件不可读、`--scope` 误用）：exit 2
- 结果以 JSON 输出到 stdout（`list` 例外，输出纯文本列表；此命令组不受 `--json` 影响）；错误输出为 `barista: <msg>`（无 CODE、无 hint），与其他命令组的全局格式不同

## 附录：JSON 输出示例

git 命令组（`barista git status --json`）：

```json
{
  "schemaVersion": 1,
  "command": "git status",
  "workspace": "D:/work/myworkspace",
  "success": true,
  "summary": { "total": 1, "ok": 1, "skipped": 0, "failed": 0 },
  "results": [
    {
      "name": "order-service",
      "path": "repos/order-service",
      "status": "ok",
      "action": "status",
      "branch": "main",
      "detail": { "ahead": 0, "behind": 0, "staged": 0, "modified": 1, "untracked": 0, "changed": true },
      "durationMs": 12
    }
  ]
}
```

错误 envelope（任意命令，exit 2）：

```json
{
  "schemaVersion": 1,
  "command": "git status",
  "success": false,
  "error": { "code": "WORKSPACE_NOT_FOUND", "message": "no .barista directory found in current directory or any parent" }
}
```

`mvn --dry-run --json` 的 `results[0].detail` 含解析明细：

```json
{
  "maven": { "name": "maven-3.9", "version": "3.9.9", "home": "...", "bin": "...", "source": "user" },
  "jdk": { "spec": "17", "name": "temurin17", "version": "17.0.13", "javaHome": "...", "source": "workspace" },
  "launch": { "value": "script", "source": "default" },
  "args": ["clean", "install"],
  "command": "cmd /c .../bin/mvn.cmd clean install"
}
```
