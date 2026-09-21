# workspace 域详细设计

> 全局契约见 [../architecture.md](../architecture.md)；本文档是 workspace 域（`internal/workspace`、`cli/init`、`cli/repo`，及配置分层细节）的详细设计，改动该域前必读。

## internal/workspace 包职责

- 工作区发现：向上找 `.barista/`（FindWorkspaceRoot，含 home 守卫）
- `repos.json` 与两级 `config.json` 的加载、合并与校验
- `repos.json` 追加写（AddRepo）与注销（RemoveRepo）
- `properties.json` 读写（SetProperty）
- cwd → repo 匹配（MatchRepo，最长前缀）
- 上爬查找（FindUpward / FindGitRoot，兼容 `.git` 文件与目录两种形态；upward.go 检出根判定）
- `.java-version` 原子写（WriteJavaVersionFile）
- init：skeleton 创建与两层检出扫描（ScanCheckouts）

## 工作区发现与 home 守卫

- 命令的 workspace 发现是机会主义的：FindRoot 找不到不算错误，仅意味着无 workspace 级覆盖
- **home 目录守卫**：向上找到的 `.barista` 若就是 user 级 `~/.barista`（root == 用户主目录），不视为 workspace——防止把 workspace 偏好写进 user 级文件。守卫实现收敛在 `workspace.FindWorkspaceRoot`，maven / gradle 组与各执行器共用

## init 命令（cli/init）

- `barista init [path]`：`workspace.Init` 创建 `.barista/repos.json` skeleton；已存在永不覆盖，报 already initialized；目标路径不存在报 CONFIG_ERROR
- `--scan`：经 `workspace.ScanCheckouts` 扫两层内（`*/.git`、`repos/*/.git`，跳过隐藏目录与 `.barista`）的检出，按 origin URL 逐条 AddRepo 导入；无 origin / 已注册 / name 冲突均跳过并记录原因；导入的 URL 同样走 baseUrl 转相对

## repo 命令组（cli/repo）

管理工作区清单 repos.json，是唯一**写** repos.json 的入口（`init --scan` 的导入也复用同一 AddRepo 路径）；自带轻骨架（单仓库操作不走 runner / panel）。

- `repo add <url>` 先注册后克隆：
  - 注册走 `workspace.AddRepo`：原子写，保留未知顶层字段与既有条目原始字节；baseUrl 前缀自动剥成相对存储；name 存在且 URL 等价 → 幂等跳过，URL 不同 → REPO_EXISTS（exit 2）
  - 克隆复用 `gitrun.Clone`：目标已是 git 仓库先校验 origin 与 ResolvedURL 一致，不符报 REPO_REMOTE_MISMATCH（exit 1）
  - clone 失败留下"已注册未检出"的合法状态，重跑自动续 clone
- URL 等价比较统一走 `workspace.NormalizeURL`（去尾部 `/` 与 `.git`）
- `repo list` 只读：按声明顺序列出清单条目，检出状态以 `<path>/.git` 存在性判定（不起子进程）
- `repo remove`：经 `workspace.RemoveRepo` 注销条目（同一原子写契约）；默认保留检出；`--delete` 仅当目标是 git 检出才删目录；确认契约同 uninstall（TTY 询问 / 非 TTY CONFIRMATION_REQUIRED / `--yes`）

## repos.json

纯事实清单：baseUrl、defaultBranch、repos。

- repo 条目的 `deps`：声明本 repo 构建前需先构建的清单内 repo 名（稳定事实，人维护），供 `barista deps` 消费（见 [deps.md](deps.md)）
- repo 条目的 `properties` map（string→string）：承载 **per-repo 覆盖**。当前被读取的键：
  - `jdk`：工具链声明（不限 maven），cwd 命中 repo 时优先于 workspace jdk
  - `maven.launch`：java | script，同理
  - 未知键宽容忽略，其他域可复用同一容器

## properties.json

`<workspace>/.barista/properties.json`：workspace 级偏好 KV map（点分键，Java properties 风格；值允许 string / number / boolean 标量）。**仅 workspace 级存在**，user 级无此文件；缺失视为空。

当前被读取的键：

| 键 | 说明 |
| --- | --- |
| `jdk` | 跨域公用工具链声明，由 `barista jdk use` 写入 |
| `maven.default` | 由 `barista maven set-default --scope workspace` 写入 |
| `maven.repo.local` | `barista mvn` 显式设置时注入 `-Dmaven.repo.local`（见 [executors.md](executors.md)） |
| `maven.launch` | mvn 启动模式覆盖（java \| script，见 [executors.md](executors.md)） |
| `gradle.default` | 由 `barista gradle set-default --scope workspace` 写入 |
| `gradle.user.home` | `barista gradle` 注入 `--gradle-user-home`（见 [executors.md](executors.md)） |
| `detect.files` | `false` 关闭 mvn / java / gradle 的 `.java-version` 与 wrapper 文件检测；手改写入 |
| `git.fetch.prune` / `git.pull.rebase` | `barista git fetch` / `git pull` 未显式传 flag 时读取；手改写入 |

未知键宽容忽略；CLI 写入走 `workspace.SetProperty`（通用 map 读写 + 临时文件 rename 原子写，未知键全保留）。

## config.json（两级）

- 顶层字段只剩 `parallel` / `color` / `colorProfile`（properties 已拆出为独立文件）
- `color`：`auto|always|never`，何时着色（`NO_COLOR` 环境变量优先级最高）
- `colorProfile`：`auto|truecolor|256|16`，着色时的色深；`auto` 按终端能力探测并自动降级，其余值强制（配合 `color: always` 可向管道/CI 日志输出指定色深）
- workspace 级 `<workspace>/.barista/config.json` 与 user 级同 schema，覆盖 user 级

## installDir

- 归属各自 registry 顶层字段（jdk.json / maven.json / gradle.json），由 `barista jdk set-install-dir` / `barista maven set-install-dir` / `barista gradle set-install-dir` 写入（须为绝对路径，`~` 允许；`--reset` 清空字段恢复内置默认 `~/.barista/toolchains/jdk|maven|gradle`）
- 不参与 workspace 合并
- 旧 config.json 残留的 `installDir` / `mavenInstallDir` 已废弃，按未知字段宽容忽略
