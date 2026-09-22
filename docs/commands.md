# 命令参考

barista CLI 的完整命令参考。全局设计契约（分层、输出、退出码）见 [architecture.md](architecture.md)；各域详细设计见 [design/](design/)。

## 全局约定

### 全局 flags

| flag         | 默认 | 说明                                 |
| ------------ | ---- | ------------------------------------ |
| `--json`     | off  | 结果以 JSON envelope 输出到 stdout   |
| `--parallel` | 10   | 并发执行的仓库/任务数上限            |

`--parallel` 未显式给出时回退到 config 的 `parallel` 字段（git 命令组读 user+workspace 合并后的 config；`jdk discover` 只读 user 级 config）。

文本输出的颜色由两级 config 的 `color`（`auto|always|never`，何时着色；`NO_COLOR` 环境变量优先于一切）与 `colorProfile`（`auto|truecolor|256|16`，色深；`auto` 按终端探测自动降级，其余值强制）控制。

下载镜像由 4 个 config 键控制（user 级与 workspace 级 config.json 合并、workspace 覆盖 user，与 `parallel` 同一契约）：

| 键                      | 取值                                                        | 作用域                          |
| ----------------------- | ----------------------------------------------------------- | ------------------------------- |
| `download.mirror`       | `official` / 预设名（`cn` `tuna` `huawei` `tencent`）       | 三域默认                        |
| `jdk.download.mirror`   | 同上（仅预设，不接受自定义 URL）                            | `jdk install` / `jdk download`，仅 temurin 生效 |
| `maven.download.mirror` | 预设名或自定义 `https://` 基址                              | `maven install`                 |
| `gradle.download.mirror`| 预设名或自定义 `https://` 基址                              | `gradle install`                |

分域键优先于全局键；`jdk install` / `jdk download` / `maven install` / `gradle install` 的 `--mirror` flag 显式给出时优先级最高（覆盖两级 config；`--mirror official` 临时禁用已配置的镜像）。镜像只替换二进制下载基址，版本元数据与校验和恒走官方；镜像不可达 / 404 时自动回退官方源一次（回退时 stderr 提示完整官方 URL），校验失败不静默回退（报 `*_CHECKSUM_MISMATCH`，hint 指向镜像配置）。详见 [design/mirror.md](design/mirror.md)。

fang 框架另自带隐藏的 `man` 命令（生成 manpages）与 root 的 `--version` flag。

### 输出形态

- text：非 TTY 逐条输出；TTY 下 git 命令组渲染实时面板
- JSON：`--json` 时输出 envelope，含 `schemaVersion: 1`、`command`、`success`、`results`；结果顺序与声明顺序一致
- 错误：stderr 输出 `barista: CODE: message`（可带 `hint:` 行），`--json` 时 stdout 另出 error envelope
- 例外：`jdk`/`maven`/`gradle` 的 `which`（非 `--pathonly`）恒输出 JSON envelope，不受 `--json` 影响；`schema` 组只有 `show`（schema 原文）与 `validate`（校验结果）恒输出 JSON（`list` 与裸 `barista schema` 输出纯文本列表），其错误格式为 `barista: <msg>`（无 CODE、无 hint）；jdk/maven/gradle 的 `path`/`home`/`which --pathonly` 输出纯路径，无尾随换行
- jdk/maven/gradle 单目标命令（add/remove/set-default/set-jdk/use/install/uninstall）：text 模式下结果级失败以 `failed <name>: CODE: message`（可带 `hint:` 行）输出到 stderr；`--json` 时失败只体现在 envelope 的 results 中

### 退出码

| 码  | 含义                                               |
| --- | -------------------------------------------------- |
| 0   | 全部成功                                           |
| 1   | 至少一个结果失败（含 schema 校验不通过、mvn/gradle 子进程非零退出、which 解析失败） |
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

- `--repo` / `--label`（git / deps 组）：当前工作区的仓库名与标签
- jdk 组位置参数：`which`/`path`/`home`/`env`/`use` 补 name + major；`remove`/`uninstall` 补 name；`set-default` 第一段补 major、第二段补 name
- maven 组位置参数：`which` 补 name + version；`remove`/`uninstall`/`set-default` 补 name；`set-jdk` 补 JDK spec
- gradle 组位置参数：`which`/`path`/`home` 补 name + version；`remove`/`uninstall`/`set-default` 补 name；`set-jdk` 补 JDK spec
- `mvn --jdk` / `java --jdk` / `gradle --jdk`：JDK spec
- `jdk env --shell`：sh/cmd/powershell/pwsh/ps（bash/zsh 是输入别名，不提供为补全候选）
- `jdk download --os` / `--arch`：linux/darwin/windows 与 amd64/arm64 静态枚举；`--output`：目录
- `schema show` / `validate` 第一段：schema 名；`init [path]` 与 `repo add --path`：目录；`repo remove` 第一段：清单中的仓库名

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
| `list`            | 按声明顺序列出清单仓库及检出状态（别名 `ls`） |
| `remove <name>`   | 注销清单条目（默认保留检出；`--delete` 连带删除） |

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

### list 要点

- 按 repos.json 声明顺序输出；检出状态用 `<path>/.git` 存在性判定（不起子进程、不联网），未检出标记 `not cloned`
- text 列为 NAME / PATH / URL（resolvedUrl）/ LABELS / CHECKOUT；JSON detail 含 `url` / `resolvedUrl` / `cloned` 及可选的 `labels` / `defaultBranch` / `deps` / `properties`
- exit 0；无清单条目时 text 打印提示、JSON 为空 results

### remove 要点

- 默认仅注销清单条目，保留磁盘检出，无确认（同 `jdk remove` 语义）；name 不在清单报 `REPO_NOT_FOUND`（exit 1）
- `--delete` 连带删除检出目录：仅当目标是 git 检出（`.git` 存在）才删，防止误删普通目录（不符报 `NOT_CLONED` exit 1）；确认契约同 uninstall——TTY 询问 / 非 TTY `CONFIRMATION_REQUIRED`（exit 2）/ `--yes` 直通；TTY 下回答 no 标记 skipped/aborted
- 写 repos.json 为原子写，保留未知顶层字段与其余条目的原始字节
- exit 0 成功（含 TTY 取消）/ 1 未找到条目或删除失败 / 2 用法、配置与确认类错误

```bash
barista repo list
barista repo remove order-service --delete --yes
```

## deps — 仓库依赖图

从 `.barista/repos.json` 的 `repos[].deps` 声明读取仓库间依赖关系。纯清单操作：只读、离线、不要求任何 repo 已检出。裸 `barista deps` 显示帮助。

```txt
barista deps show  [--repo <name>...] [--label <label>...]
barista deps order [--repo <name>...] [--label <label>...]
```

| 命令   | 行为                                                       |
| ------ | ---------------------------------------------------------- |
| `show` | 邻接表：每个 repo 依赖谁、dangling 引用与环标注            |
| `order` | 构建层级序：层级 = 1 + 依赖链最长路径，同层可并行构建      |

### 数据声明

```json
{ "name": "order-service", "url": "order-service", "deps": ["common-lib", "user-service"] }
```

- `deps` 为本 repo 构建前需先构建的清单内 repo 名（稳定事实，人维护）；缺省/空 = 无内部依赖
- 引用不存在的 repo 名是 dangling：deps 命令如实标注（不报错），`barista doctor` 的 `repoDeps` 检查报 failed

### 要点

- 选择器 `--repo` / `--label` 语义与 git 组一致；图始终在完整清单上构建，选择器只过滤输出（子集内 repo 的层级与全量一致）
- `order` 同层 repo 按 repos.json 声明顺序显示（仅为确定性，不携带构建语义）；环成员折叠同层，环以 `cycle: <成员列表>` 标注
- 无失败路径：环与 dangling 是事实展示而非失败，exit 恒 0；仅用法/配置错误 exit 2
- JSON：results 按声明顺序（`order` 也不例外），层级在 detail `buildLevel`；`show` 的 detail 含 `deps` / `dependedBy`，以及可选的 `dangling` / `cycle`

```bash
barista deps show
barista deps order --label java
```

## jdk — JDK 注册表管理

注册表为 user 级 `~/.barista/jdk.json`。托管安装目录默认 `~/.barista/toolchains/jdk/`（jdk.json 顶层 `installDir` 字段，用 `barista jdk set-install-dir` 设置）。`use` 是 jdk 组唯一需要工作区的命令。

### 子命令

| 命令                         | 行为                                                        |
| ---------------------------- | ----------------------------------------------------------- |
| `discover`                   | 扫描 JAVA_HOME 系 env / sdkman / 平台安装位置 / PATH 并注册 |
| `add <name> <path>`          | 注册已安装的 JDK（起 `java` 探测版本与发行版；`javac` 仅做存在性检查） |
| `install <distro><major>`    | 下载并安装到托管目录（支持 temurin / microsoft / corretto / zulu / graalvm） |
| `download <distro><major>`   | 只下载归档不安装（`--os` / `--arch` 跨平台，`--output` 指定目录或文件路径） |
| `available`                  | 列出远端可安装的 JDK（major / 最新版本 / LTS / 是否已装）   |
| `list`                       | 列出已注册 JDK（别名 `ls`）                                 |
| `which <major\|name>`        | 解析 JDK 并打印信息（恒输出 JSON）                          |
| `path` / `home <major\|name>` | 只打印路径（`which --pathonly` 的快捷方式）                 |
| `env <major\|name>`           | 打印 JAVA_HOME/PATH 导出语句（`--shell sh\|cmd\|powershell`，缺省自动检测当前 shell，供 eval / CI 消费） |
| `set-default <major> <name>` | 设置某个 major 版本的默认 JDK                               |
| `set-install-dir <path>`     | 设置托管安装根目录（写入 jdk.json `installDir`；`--reset` 恢复内置默认） |
| `use <major\|name>`          | 设置当前工作区的 JDK（写 workspace `.barista/properties.json` 的 `jdk` 键；cwd 位于 git 检出内时同时写 `<检出根>/.java-version`） |
| `remove <name>`              | 仅注销，保留磁盘文件（级联清理指向它的 defaults）           |
| `uninstall <name>`           | 删除 managed 安装并注销（级联清理 defaults）                |

### 要点

- discover：幂等可重复；已注册路径报 skipped；是唯一走 `--parallel` 并发的 jdk 命令（每个候选路径一个探测任务）
- add：名称必须匹配 `[a-z0-9][a-z0-9._-]*` 且不能是纯数字（避免与 major 版本解析歧义），不符按用法错误处理（exit 2）；`--default` 同时设为该 major 的默认
- install：命名即 `<distro><major>`；下载支持断点续传与指数退避重试（默认 10 次，`--attempts` 覆盖，必须 >= 1），TTY 下 stderr 渲染进度条；`--mirror` 显式覆盖 config 的镜像配置（仅 temurin 生效，值域为 official / 预设名，`official` 临时禁用镜像）；JSON detail 恒带 `downloadUrl` 记录实际下载来源（镜像生效时另带 `mirror`）；嵌入数据的 distro（zulu / graalvm）带 sha256 钉值，下载后校验，不符报 `JDK_CHECKSUM_MISMATCH`；解压后 probe 校验 major 匹配才注册为 `managed: true`，任何失败清理半成品目录
- download：只下载不安装——不读注册表、不解压、不 probe、不注册；`--os` / `--arch` 缺省当前平台，可跨平台下载（非法值报用法错误 exit 2）；文件名取 Content-Disposition 或最终 URL basename，拿不到回退 `<distro>-jdk-<major>-<os>-<arch>`+扩展名（distro 名含 jdk 时省略 `-jdk` 段）；`--output` 缺省当前目录，支持 `~` 展开，值是已存在目录或以路径分隔符结尾时作为目录拼接文件名，否则视为完整文件路径；目标已存在报 `JDK_EXISTS`（exit 1）；下载写 `<dest>.part` 成功后 rename，`.part` 留存时重跑天然断点续传；`--attempts` 设置下载尝试次数（默认 10，必须 >= 1）；`--mirror` 显式覆盖 config 的镜像配置（同 install）；JSON detail 的 `url` 为实际下载来源（回退后为官方 URL），镜像生效时另带 `mirror`；有 sha256 钉值时校验，不符删 `.part` 报 `JDK_CHECKSUM_MISMATCH`
- available：列出各 distro 可安装项；数据源按 distro 而异——temurin 实时查 Adoptium API（用户显式调用才联网），microsoft / corretto 为静态 major 列表（version 留空，permalink 始终指向最新 GA），zulu / graalvm 读嵌入二进制的 distros.json（离线，由 `scripts/gendistros.py` 定期刷新）；按 distro + major 升序输出，tags 标记 `lts` / `latest`（最新 LTS）/ `installed`（对照注册表）/ `unsupported-platform`；网络失败报 `JDK_AVAILABLE_FAILED`（exit 1）
- which 解析：传 major 先取该 major 的 default，否则取最新；传 name 精确匹配；非 `--pathonly` 时恒输出 JSON envelope（不受 `--json` 影响），解析失败 exit 1
- env：与 which/path 同款解析；默认输出 JAVA_HOME 与 PATH（前置 `<jdk>/bin`）导出语句；`--shell` 支持别名（bash/zsh→sh，pwsh/ps→powershell），缺省自动检测当前 shell（Windows 按父进程名，其次 MSYSTEM/SHELL 环境标记；Unix 读 `$SHELL`），检测不到回退平台默认（Windows → powershell，其余 → sh）；`--json` 时改输出 envelope（含 javaHome/bin/shell），解析失败 exit 1
- remove：直接注销，无确认、不删文件（幂等），并级联清理 jdk.json 中所有指向该 JDK 的 defaults
- uninstall：仅作用于 managed 条目；确认契约为 TTY 交互询问，非 TTY 报 `CONFIRMATION_REQUIRED`（exit 2），`--yes` 直通
- use：除写 workspace properties.json 外，cwd 上爬命中 `.git`（文件或目录形态均可，边界为 workspace 根，取最近的嵌套检出）时，向该检出根原子写 `.java-version`，内容为解析后条目的精确版本（如 `17.0.13`）；已存在且值不同则覆盖并在 text 输出提示 `updated <path>/.java-version (<old> → <new>)`；不依赖 repos.json 注册（手工克隆的检出同样生效）；不在检出内则行为不变；写入失败结果降级 failed（CONFIG_ERROR），properties 不回滚；不受 `detect.files` 开关影响

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
| `available`             | 列出 Apache archive 上可下载的版本（`--all` 列出全部 patch）            |
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

- add / discover / install 的自动命名为 `maven-<version>`（完整版本号含 qualifier，如 `maven-3.9.11` / `maven-4.0.0-rc-4`），冲突自动追加 `-1` / `-2`；add 与 install 均支持 `--name` 指定注册名，名称必须匹配 `[a-z0-9][a-z0-9._-]*` 且不能形如版本号（如 `3.9` / `3.9.9`，避免与版本解析歧义），不符按失败结果报 CONFIG_ERROR；install 的 `--name` 与已注册名冲突报 MAVEN_EXISTS；add 的 `--default` 同时设为默认
- set-default：对 name 校验同一名称模式 `[a-z0-9][a-z0-9._-]*`（不符为用法错误，exit 2），但不查版本号形态（remove/uninstall/set-default 本就强制精确名，不走模糊解析）
- install：先查 Apache archive 可用版本，无完全匹配时按数字段前缀放宽——最长前缀优先，同一前缀层级内稳定版优先于预发布，再取最高者（`MatchAvailable`，与 Resolve 放宽语义一致）；替换在下载前告知，TTY 下交互确认（`[Y/n]` 默认 yes，回答 n 中止为 skipped、exit 0），非 TTY / `--json` / `--yes` 不询问只警告（JSON detail 带 `requestedVersion`；detail 恒带 `downloadUrl` 记录实际下载来源，镜像生效时另带 `mirror`）；`--attempts` 设置下载尝试次数（默认 10，必须 >= 1）；`--mirror` 显式覆盖 config 的镜像配置（official / 预设名 / 自定义 `https://` 基址，`official` 临时禁用镜像）；完全无匹配报 MAVEN_NOT_FOUND；解压后 probe 校验版本与解析结果完全一致，失败清理目录
- available：抓取 archive.apache.org 目录列表（与 install 同一来源），默认每个 minor 线只列最新版本，`--all` 列全部；text 输出 VERSION/TAGS 表（latest / installed 标记，installed 按版本与注册表比对）；联网失败报 MAVEN_AVAILABLE_FAILED
- which 版本解析逐级放宽：`3.9.9` → `3.9` → `3`；非 `--pathonly` 时恒输出 JSON envelope，解析失败 exit 1
- 生效优先级：workspace 配置 `maven.default` / `jdk` 覆盖 user 注册表字段
- set-jdk：只写 user 级 maven.json，写入的是用户给的原始 spec（如 `17`），而非解析后的注册名；workspace 级覆盖用 `barista jdk use`
- remove/uninstall 契约与 jdk 组相同：remove 直接注销无确认；uninstall 仅 managed，TTY 询问 / 非 TTY exit 2 / `--yes` 直通；两者若为 user 级 default 均一并清除该 default

```bash
barista maven available
barista maven install 3.9.9
barista maven set-default maven-3.9.9
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

- Maven 安装：`.mvn/wrapper/maven-wrapper.properties` 的 `distributionUrl` 版本 > workspace properties.json `maven.default` > user maven.json default（无 repo 级）
- JDK：`--jdk` > `.java-version` 文件 > repo `properties["jdk"]` > workspace properties.json `jdk` > user maven.json jdk > ambient（不动 JAVA_HOME/PATH）
- settings.xml：存在 `.barista/maven/settings.xml` 时注入 `-s`（自行传 `-s`/`--settings` 则跳过）；同目录 `settings-security.xml` 存在时配套注入 `-Dsettings.security`（自行传 `-s`/`--settings` 或 `-Dsettings.security` 则跳过）
- 本地仓库：workspace properties.json `maven.repo.local` 注入 `-Dmaven.repo.local`（自行传则跳过）；相对路径基于 workspace 根，支持 `~` 展开
- launch：`--launch` > repo `maven.launch` > workspace properties.json `maven.launch` > `script`

### 文件检测（`.java-version` 与 wrapper）

- `.java-version`：从 cwd 逐级上爬查找，边界为最近的 `.git` 检出根（worktree 的 `.git` 文件形态兼容，不依赖 repos.json 注册）；无检出根时边界为 workspace 根，workspace 外则只在 cwd 单点查找。内容支持 `17` / `17.0.13` / `1.8`（→ major 8）及 distro 词元（`temurin-17`、`temurin@17`、`17.0.13-tem`，别名 tem/ms/cor/zul/grl 映射全称）；distro 词元命中时先试注册名 `<distro><major>`（如 `temurin17`），落空回退 major 解析；声明的版本未注册响亮报 `JDK_NOT_FOUND`（exit 2，message 带来源）；内容无法解析时视为该级不存在，继续回退
- wrapper：在 `findBasedir` 命中的 basedir（感知 `MAVEN_BASEDIR` / `-f` / 上爬 `.mvn`）下读 `.mvn/wrapper/maven-wrapper.properties`，从 `distributionUrl` 提取 `apache-maven-<version>-bin.` 版本，经注册表逐级放宽解析；未注册响亮报 `MAVEN_NOT_FOUND`（hint 指向 `barista maven install <version>`）
- 退出开关：workspace properties.json 键 `detect.files=false` 关闭上述两类文件检测（不影响 `jdk use` 的 `.java-version` 写入）；无 workspace 时开关不存在、视为开启
- `--dry-run` 输出中来源为文件的层级带 `file` 字段（text 与 JSON 均体现）

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

- JDK：`--jdk` > `.java-version` 文件 > repo `properties["jdk"]` > workspace properties.json `jdk` > ambient（PATH 中的 java，不动 JAVA_HOME/PATH）
- `.java-version` 的查找边界、格式与失败语义与 `barista mvn` 一致（见上文"文件检测"小节，含 `detect.files=false` 开关）
- 显式层级（flag/文件/repo/workspace）解析不到注册 JDK 时响亮报 `JDK_NOT_FOUND`（exit 2，message 带来源），不静默回退
- 无任何声明且 PATH 无 java 时报 `JDK_NOT_FOUND`（exit 2）

### 其他行为

- 解析到注册 JDK 时直接执行其 `bin/java`（Windows 为 `bin/java.exe`，不经 cmd 包装），子进程 JAVA_HOME 被替换为该 JDK
- java 子进程非零退出时 barista exit 1；stdout/stderr 直接继承
- 缺 `--` 直接给参数报 `USAGE_ERROR`（exit 2）；`--dry-run` 的 `--json` 输出含 jdk / bin / args / command 明细

```bash
barista java -- -version
barista java --jdk 17 --dry-run -- -jar app.jar
```

## gradle — Gradle 注册表管理与执行器

注册表为 user 级 `~/.barista/gradle.json`。托管安装目录默认 `~/.barista/toolchains/gradle/`（gradle.json 顶层 `installDir` 字段，用 `barista gradle set-install-dir` 设置）。`set-default` 支持 `--scope user|workspace`（默认 user；workspace 写入 `<workspace>/.barista/properties.json`）；`set-jdk` 只写 user 级（workspace 级 JDK 用 `barista jdk use`）。

组根本身是执行器：`barista gradle [flags] -- <gradle args...>` 在当前目录运行 Gradle；下列管理子命令与透传参数同名时子命令优先（如 `barista gradle install 8.10.2` 走 install 子命令）。

### 子命令

| 命令                     | 行为                                                                       |
| ------------------------ | -------------------------------------------------------------------------- |
| `discover`               | 扫描 GRADLE_HOME / sdkman / brew / scoop / 平台位置 / PATH 并注册          |
| `add <path>`             | 注册已安装的 Gradle（从 `lib/gradle-core-*.jar` 文件名读版本，不起子进程） |
| `install <version>`      | 从 services.gradle.org 下载 bin 发行包（sha256 校验）安装到托管目录        |
| `available`              | 列出 services.gradle.org 上可下载的版本（`--all` 含 rc/milestone 预发布与全部历史版本） |
| `list`                   | 列出已注册 Gradle（别名 `ls`）                                             |
| `which [name\|version]`  | 按名或版本解析（版本逐级放宽）；不传参数取生效默认（恒输出 JSON）          |
| `path` / `home`          | 只打印 gradle home 路径（`which --pathonly` 的快捷方式）                   |
| `set-default <name>`     | 设置默认 Gradle（`--scope` 选择写入层级）                                  |
| `set-jdk <major\|name>`  | 设置运行 Gradle 的 JDK（按 jdk 注册表解析，写 user gradle.json）           |
| `set-install-dir <path>` | 设置托管安装根目录（写入 gradle.json `installDir`；`--reset` 恢复内置默认） |
| `config`                 | 查看生效配置（default / jdk / installDir 及 workspace 级项）及其来源       |
| `remove <name>`          | 仅注销，保留磁盘文件（若为 user 级 default 一并清除）                      |
| `uninstall <name>`       | 删除 managed 安装并注销（同样清除指向它的 user 级 default）                |

### 执行器用法

```txt
barista gradle [flags] -- <gradle args...>
```

| flag        | 说明                                              |
| ----------- | ------------------------------------------------- |
| `--jdk`     | JDK spec（注册名或 major 版本），覆盖所有配置层级 |
| `--dry-run` | 打印解析出的环境与完整命令行，不执行              |

- `--` 后参数原样透传；缺 `--` 直接给 task 报 `USAGE_ERROR`（exit 2）；无参数且未给 flag 时显示帮助

### 解析链（高 → 低）

- Gradle 安装：`gradle/wrapper/gradle-wrapper.properties` 的 `distributionUrl` 版本 > workspace properties.json `gradle.default` > user gradle.json default（无 repo 级）
- JDK：`--jdk` > `.java-version` 文件 > repo `properties["jdk"]` > workspace properties.json `jdk` > user gradle.json jdk > ambient（不动 JAVA_HOME/PATH）
- init script：`.barista/gradle/init.gradle` 与 `.barista/gradle/init.gradle.kts` 存在即各注入 `-I`（自行传 `-I`/`--init-script` 则跳过）
- gradle user home：workspace properties.json `gradle.user.home` 注入 `--gradle-user-home`（自行传 `-g`/`--gradle-user-home`/`-Dgradle.user.home` 则跳过）；相对路径基于 workspace 根，支持 `~` 展开

### 文件检测（`.java-version` 与 wrapper）

- `.java-version`：查找边界、格式与失败语义与 `barista mvn` 一致（见 mvn 章"文件检测"小节）
- wrapper：从 cwd 逐级上爬定位 `gradle/wrapper/gradle-wrapper.properties`，边界为最近的 `.git` 检出根（无 `MAVEN_BASEDIR` 对应物）；从 `distributionUrl` 提取 `gradle-<version>-bin|all.zip` 的版本，经注册表逐级放宽解析；未注册响亮报 `GRADLE_NOT_FOUND`（hint 指向 `barista gradle install <version>`）
- 退出开关：workspace properties.json 键 `detect.files=false` 关闭上述两类文件检测（与 mvn / java 共用）
- `--dry-run` 输出中来源为文件的层级带 `file` 字段（text 与 JSON 均体现）

### 要点

- 只有 script 启动：Unix 直接 `bin/gradle`，Windows 经 `cmd /c bin/gradle.bat`（无 mvn 的 `--launch` 对应物）；解析到 JDK 时子进程 JAVA_HOME 被替换为该 JDK；Gradle 子进程非零退出时 barista exit 1
- add / discover / install 的自动命名为 `gradle-<version>`（完整版本号含 qualifier，如 `gradle-9.0.0-rc-1`），冲突自动追加 `-1` / `-2`；`--name` 规则同 maven 组（必须匹配 `[a-z0-9][a-z0-9._-]*` 且不能形如版本号）；add 的 `--default` 同时设为默认
- 固有局限：预发行版发行包的 jar 用裸基础版本命名（probe 不起子进程，读不出 qualifier），add / discover 对预发行版 home 只能注册基础版本（如 `9.0.0`）；install 不受此限，注册完整版本号（如 `9.0.0-rc-1`）
- install：先经 available 列表解析版本，无完全匹配按数字段前缀放宽——最长前缀优先，同一前缀层级内稳定版优先于预发布，再取最高者；替换在下载前告知，TTY 下交互确认（`[Y/n]` 默认 yes，回答 n 中止为 skipped、exit 0），非 TTY / `--json` / `--yes` 不询问只警告（JSON detail 带 `requestedVersion`；detail 恒带 `downloadUrl` 记录实际下载来源，镜像生效时另带 `mirror`）；`--attempts` 设置下载尝试次数（默认 10，必须 >= 1）；`--mirror` 显式覆盖 config 的镜像配置（official / 预设名 / 自定义 `https://` 基址，`official` 临时禁用镜像）；sha256 优先取 `/versions/all` 内联 checksum，缺省时下载 checksumUrl 旁挂文件，不匹配报 `GRADLE_CHECKSUM_MISMATCH`
- available：数据源 `services.gradle.org/versions/all`，过滤 snapshot / nightly / releaseNightly / broken；默认只列两个最新 major 线各 minor 的最新 final 版本（预发布只随 `--all` 出现）；tags 标记 latest / installed / rc / milestone；联网失败报 `GRADLE_AVAILABLE_FAILED`
- which 版本解析逐级放宽：`8.10.1` → `8.10` → `8`；非 `--pathonly` 时恒输出 JSON envelope，解析失败 exit 1
- 生效优先级：workspace 配置 `gradle.default` / `jdk` 覆盖 user 注册表字段
- set-jdk：只写 user 级 gradle.json，写入的是用户给的原始 spec（如 `17`），而非解析后的注册名；workspace 级覆盖用 `barista jdk use`
- remove/uninstall 契约与 maven 组相同：remove 直接注销无确认；uninstall 仅 managed，TTY 询问 / 非 TTY exit 2 / `--yes` 直通；两者若为 user 级 default 均一并清除该 default
- `--dry-run` 的 `--json` 输出 detail 含 gradle / jdk / args / command 及可选的 workspace / repo / initScripts / gradleUserHome 解析明细

```bash
barista gradle available
barista gradle install 8.10.2
barista gradle set-default gradle-8.10.2
barista gradle -- build test
barista gradle --jdk 17 --dry-run -- -q assemble
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

- user 级：git 在 PATH；`JAVA_HOME` 有效性（未设置是合法的 ambient 状态，报 skipped）；`config.json` / `jdk.json` / `maven.json` / `gradle.json` 可解析；每个 JDK 条目 `bin/java` 存在、每个 Maven / Gradle 安装 probe 版本与注册一致（纯文件系统）；`defaults` / `default` / `jdk` / `installDir` 引用可解析
- workspace 级：`.barista` 整体可加载（repos.json / config.json / properties.json）；每个 repo 检出存在（未克隆报 skipped，含补救命令）；`repos[].deps` 引用完整性（dangling 报 failed，`repoDeps`）；workspace properties 的 `jdk` / `maven.default` / `maven.launch` / `gradle.default` 与 per-repo properties 的 `jdk` / `maven.launch`（无 per-repo `maven.default` / `gradle.default`，与 mvn / gradle 解析链"无 repo 级"一致）可解析或合法；`settings.xml` / `settings-security.xml` 与 `.barista/gradle/` 下 `init.gradle` / `init.gradle.kts` 存在性（不存在报 skipped，可选文件，两个 init script 各自独立报告）；逐 repo 浅查 `.java-version`（`javaVersionFile`：存在则校验可解析且注册表可解析，distro 词元逻辑同 mvn/java）与 `.mvn/wrapper/maven-wrapper.properties`（`mavenWrapperFile`：存在则校验 `distributionUrl` 版本注册表可解析，失败 hint 指向 `barista maven install <version>`）、`gradle/wrapper/gradle-wrapper.properties`（`gradleWrapperFile`：同上语义，失败 hint 指向 `barista gradle install <version>`）；文件不存在均报 skipped。注意 wrapper 检查只看 repo 检出根，不上爬、不读 `MAVEN_BASEDIR`（与 mvn / gradle 执行器的上爬查找语义不同）

### 要点

- 浅查项装配期同步执行，probe 与 repo 检查走 runner 并发（`--parallel` 生效）；输出顺序为同步检查按声明序在前、并发检查结果按声明序追加在后
- 结果三态：ok / skipped（不适用或信息项，reason 在 detail）/ failed（带 hint）；exit 0 全过 / 1 有 failed / 2 用法错误
- `--json` 每项 detail 含 `scope`（user|workspace）与 `check`（检查 id，如 `jdkInstall` / `repoCheckout`）

## upgrade — 自更新

从最新 GitHub release 的清单文件（`manifest.json` asset）检测并应用自更新。所有联网均为用户显式触发：`upgrade` 与 `jdk available` / `maven available` / `gradle available` 查询版本/更新信息，`jdk install` / `jdk download` / `maven install` / `gradle install` 下载发行包，git 批量命令经系统 git 访问远端；除此之外永不被动联网（包括永不被动检测更新）。

```txt
barista upgrade [--check] [--yes] [--attempts N]
```

| flag         | 说明                                       |
| ------------ | ------------------------------------------ |
| `--check`    | 只检测是否有新版本，不下载不替换           |
| `--yes`      | 跳过确认提示（非 TTY 下必需）              |
| `--attempts` | 下载尝试次数（默认 10，必须 >= 1）         |

### 行为

- 读取 `releases/latest/download/manifest.json`（无 API 调用、无鉴权）；当前版本 ≥ 最新时报 already up to date（幂等）；dev/dirty 构建视为未知版本，始终可升级到最新 release
- 下载对应 GOOS/GOARCH 的 asset（断点续传 + 指数退避重试，TTY 下 stderr 渲染进度条），完成后 sha256 校验，不符报 `UPGRADE_CHECKSUM_MISMATCH`
- 替换当前可执行文件：Unix 临时文件 + rename 原子覆盖；Windows 先把运行中的旧 exe 改名为 `.old` 再写入新文件（`.old` 下次运行 upgrade 时自动清理）；目标不可写报 `UPGRADE_REPLACE_FAILED` 并带 hint
- 确认契约：TTY 交互询问（`[Y/n]` 默认 yes，回答 n 取消），非 TTY 报 `CONFIRMATION_REQUIRED`（exit 2），`--yes` 直通
- exit 0 已最新、升级成功或 TTY 下回答 n 取消（结果标记 skipped/aborted） / 1 网络、校验或替换失败 / 2 确认缺失等用法错误

清单文件由 release workflow 生成（六平台 asset 的 file/sha256/size），发布前用 `barista schema validate manifest <file>` 校验（不带 file 会解析到工作区默认路径 `<workspace>/.barista/manifest.json`）。

## schema — 内置 JSON Schema 工具

供 AI Agent 与编辑器在线发现、校验配置。schema 单一数据源在 `schemas/` 包。

| 命令                          | 行为                                         |
| ----------------------------- | -------------------------------------------- |
| `list`                        | 列出可用 schema（repos / config / jdk / maven / gradle / properties / manifest） |
| `show <name>`                 | 输出 schema 原文 JSON                        |
| `validate <name> [file]`      | 校验配置文件                                 |

### validate 默认文件

- 显式给 `file` 时校验该文件（此时不可再用 `--scope`）
- `jdk` / `maven` / `gradle`：默认校验对应 user 级注册表（`~/.barista/jdk.json` / `maven.json` / `gradle.json`）
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
  "maven": { "name": "maven-3.9.9", "version": "3.9.9", "home": "...", "bin": "...", "source": "user" },
  "jdk": { "spec": "17", "name": "temurin17", "version": "17.0.13", "javaHome": "...", "source": "workspace" },
  "launch": { "value": "script", "source": "default" },
  "args": ["clean", "install"],
  "command": "cmd /c .../bin/mvn.cmd clean install"
}
```
