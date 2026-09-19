# 命令参考

barista CLI 的完整命令参考。设计契约（分层、输出、退出码）见 [architecture.md](architecture.md)。

## 全局约定

### 全局 flags

| flag         | 默认 | 说明                                 |
| ------------ | ---- | ------------------------------------ |
| `--json`     | off  | 结果以 JSON envelope 输出到 stdout   |
| `--parallel` | 10   | 并发执行的仓库/任务数上限            |

`--parallel` 未显式给出时回退到 config 的 `parallel` 字段（git 命令组与 `jdk discover` 适用）。

### 输出形态

- text：非 TTY 逐条输出；TTY 下 git 命令组渲染实时面板
- JSON：`--json` 时输出 envelope，含 `schemaVersion: 1`、`command`、`success`、`results`；结果顺序与声明顺序一致
- 错误：stderr 输出 `barista: CODE: message`（可带 `hint:` 行），`--json` 时 stdout 另出 error envelope
- 例外：`jdk`/`maven` 的 `which`（非 `--pathonly`）恒输出 JSON envelope，不受 `--json` 影响；`schema` 命令组恒输出 JSON，且其错误格式为 `barista: <msg>`（无 CODE、无 hint）

### 退出码

| 码  | 含义                                               |
| --- | -------------------------------------------------- |
| 0   | 全部成功                                           |
| 1   | 至少一个结果失败（含 schema 校验不通过、mvn 子进程非零退出、which 解析失败） |
| 2   | 用法错误、配置错误、确认类错误（如 CONFIRMATION_REQUIRED） |

通用红线：stdout/stderr 严格分离；非 TTY 永不阻塞询问；一切操作幂等，可重复执行。

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

- `--prune` / `--rebase` 未显式给出时回退 repos.json 顶层 config 的 `fetch.prune` / `pull.rebase`
- checkout 三分支：本地分支已存在 → 切换；`origin/<branch>` 存在 → `checkout --track`；都没有 → 从默认分支创建（`--no-track`）
- checkout 遇未提交变更（staged/modified）报 skipped `DIRTY_WORKTREE`

```bash
barista git status --label java
barista git checkout feature/x --repo order-service --repo user-service
```

## jdk — JDK 注册表管理

注册表为 user 级 `~/.barista/jdk.json`。托管安装目录默认 `~/.barista/toolchains/jdk/`（jdk.json 顶层 `installDir` 字段，用 `barista jdk set-install-dir` 设置）。`use` 是 jdk 组唯一需要工作区的命令。

### 子命令

| 命令                         | 行为                                                        |
| ---------------------------- | ----------------------------------------------------------- |
| `discover`                   | 扫描 JAVA_HOME 系 env / sdkman / 平台安装位置 / PATH 并注册 |
| `add <name> <path>`          | 注册已安装的 JDK（起 `java` 探测版本与发行版；`javac` 仅做存在性检查） |
| `install <distro><major>`    | 下载并安装到托管目录（当前支持 temurin，走 Adoptium API）   |
| `list`                       | 列出已注册 JDK                                              |
| `which <major\|name>`        | 解析 JDK 并打印信息（恒输出 JSON）                          |
| `path` / `home <major\|name>` | 只打印路径（`which --pathonly` 的快捷方式）                 |
| `set-default <major> <name>` | 设置某个 major 版本的默认 JDK                               |
| `set-install-dir <path>`     | 设置托管安装根目录（写入 jdk.json `installDir`；`--reset` 恢复内置默认） |
| `use <major\|name>`          | 设置当前工作区的 JDK（写 workspace config.json `properties["jdk"]`） |
| `remove <name>`              | 仅注销，保留磁盘文件                                        |
| `uninstall <name>`           | 删除 managed 安装并注销（级联清理 defaults）                |

### 要点

- discover：幂等可重复；已注册路径报 skipped；是唯一走 `--parallel` 并发的 jdk 命令（每个候选路径一个探测任务）
- add：`--default` 同时设为该 major 的默认
- install：命名即 `<distro><major>`；下载支持断点续传与指数退避重试，TTY 下 stderr 渲染进度条；解压后 probe 校验 major 匹配才注册为 `managed: true`，任何失败清理半成品目录
- which 解析：传 major 先取该 major 的 default，否则取最新；传 name 精确匹配；非 `--pathonly` 时恒输出 JSON envelope（不受 `--json` 影响），解析失败 exit 1
- remove：直接注销，无确认、不删文件（幂等）
- uninstall：仅作用于 managed 条目；确认契约为 TTY 交互询问，非 TTY 报 `CONFIRMATION_REQUIRED`（exit 2），`--yes` 直通

```bash
barista jdk discover
barista jdk install temurin17
barista jdk add my-jdk8 /opt/jdk8 --default
barista jdk which 17
```

## maven — Maven 注册表管理

注册表为 user 级 `~/.barista/maven.json`。托管安装目录默认 `~/.barista/toolchains/maven/`（maven.json 顶层 `installDir` 字段，用 `barista maven set-install-dir` 设置）。`set-default` 支持 `--scope user|workspace`（默认 user；workspace 写入 `<workspace>/.barista/config.json` 的 properties）；`set-jdk` 只写 user 级（workspace 级 JDK 用 `barista jdk use`）。

### 子命令

| 命令                    | 行为                                                                   |
| ----------------------- | ---------------------------------------------------------------------- |
| `discover`              | 扫描 MAVEN_HOME/M2_HOME / sdkman / brew / scoop / 平台位置 / PATH 并注册 |
| `add <path>`            | 注册已安装的 Maven（从 `lib/maven-core-*.jar` 文件名读版本，不起子进程） |
| `install <version>`     | 从 Apache archive 下载（sha512 校验）安装到托管目录                     |
| `list`                  | 列出已注册 Maven                                                       |
| `which [name\|version]` | 按名或版本解析（版本逐级放宽）；不传参数取生效默认（恒输出 JSON）      |
| `path` / `home`         | 只打印 maven home 路径（`which --pathonly` 的快捷方式）                |
| `set-default <name>`    | 设置默认 Maven（`--scope` 选择写入层级）                               |
| `set-jdk <major\|name>` | 设置运行 Maven 的 JDK（按 jdk 注册表解析，写 user maven.json）         |
| `set-install-dir <path>` | 设置托管安装根目录（写入 maven.json `installDir`；`--reset` 恢复内置默认） |
| `config`                | 查看生效配置（default / jdk / installDir 及 workspace 级项）及其来源   |
| `remove <name>`         | 仅注销，保留磁盘文件                                                   |
| `uninstall <name>`      | 删除 managed 安装并注销                                                |

### 要点

- add：`--name` 指定注册名（默认 `maven-<major.minor>`，冲突自动追加序号）；`--default` 同时设为默认
- install：解压后 probe 校验版本与请求完全一致，失败清理目录
- which 版本解析逐级放宽：`3.9.9` → `3.9` → `3`；非 `--pathonly` 时恒输出 JSON envelope，解析失败 exit 1
- 生效优先级：workspace 配置 `maven.default` / `jdk` 覆盖 user 注册表字段
- set-jdk：只写 user 级 maven.json，写入的是用户给的原始 spec（如 `17`），而非解析后的注册名；workspace 级覆盖用 `barista jdk use`
- remove/uninstall 契约与 jdk 组相同：remove 直接注销无确认；uninstall 仅 managed，TTY 询问 / 非 TTY exit 2 / `--yes` 直通

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
| `--startup` | 启动方式：`script`（默认，mvn/mvn.cmd wrapper）或 `jar`（直接 java 启动 classworlds jar，绕开 wrapper 的引号陷阱，但也跳过 mavenrc 钩子） |
| `--dry-run` | 打印解析出的环境与完整命令行，不执行                          |

### 解析链（高 → 低）

- Maven 安装：workspace `maven.default` > user maven.json default（无 repo 级）
- JDK：`--jdk` > repo `properties["jdk"]` > workspace `jdk` > user maven.json jdk > ambient（不动 JAVA_HOME/PATH）
- settings.xml：存在 `.barista/maven/settings.xml` 时注入 `-s`（自行传 `-s`/`--settings` 则跳过）；同目录 `settings-security.xml` 存在时配套注入 `-Dsettings.security`（自行传 `-s`/`--settings` 或 `-Dsettings.security` 则跳过）
- 本地仓库：workspace `maven.repo.local` 注入 `-Dmaven.repo.local`（自行传则跳过）；相对路径基于 workspace 根，支持 `~` 展开
- startup：`--startup` > repo `maven.startup` > workspace `maven.startup` > `script`

### 其他行为

- 解析到 JDK 时子进程 JAVA_HOME 被替换为该 JDK
- Maven 子进程非零退出时 barista exit 1；stdout/stderr 直接继承
- `--dry-run` 的 `--json` 输出含 maven / jdk / startup / args / command 等解析明细

```bash
barista mvn -- clean install -DskipTests
barista mvn --jdk 17 --dry-run -- -q validate
```

## schema — 内置 JSON Schema 工具

供 AI Agent 与编辑器在线发现、校验配置。schema 单一数据源在 `schemas/` 包。

| 命令                          | 行为                                         |
| ----------------------------- | -------------------------------------------- |
| `list`                        | 列出可用 schema（repos / config / jdk / maven） |
| `show <name>`                 | 输出 schema 原文 JSON                        |
| `validate <name> [file]`      | 校验配置文件                                 |

### validate 默认文件

- 显式给 `file` 时校验该文件（此时不可再用 `--scope`）
- `jdk` / `maven`：默认校验对应 user 级注册表（`~/.barista/jdk.json` / `maven.json`）
- `config`：默认同时校验 user 与 workspace 两级；`--scope user|workspace` 限定单级
- 其他（如 `repos`）：默认校验当前工作区 `.barista/<name>.json`

### 退出码与输出

- 校验不通过：exit 1，结果 JSON 的 `errors` 列出问题
- 用法/环境错误（未知 schema、文件不可读、`--scope` 误用）：exit 2
- 结果始终以 JSON 输出到 stdout（此命令组不受 `--json` 影响）；错误输出为 `barista: <msg>`（无 CODE、无 hint），与其他命令组的全局格式不同

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
  "startup": { "value": "script", "source": "default" },
  "args": ["clean", "install"],
  "command": "cmd /c .../bin/mvn.cmd clean install"
}
```
