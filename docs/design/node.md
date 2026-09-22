# Node.js 域详细设计

> 全局契约见 [../architecture.md](../architecture.md)；本文档是 node 域（`internal/node` + `cli/node`）的详细设计，改动该域前必读。

## internal/node 包职责

- registry 读写（`~/.barista/node.json`，原子写；结构为 `installations` + `default` + `installDir`，无 jdk 字段）
- probe：布局检查（Windows 根目录 `node.exe`，Unix `bin/node`）后起子进程跑 `node --version`，解析 `vX.Y.Z` 输出（`ParseNodeVersion` 为可单测的纯函数）；与 maven/gradle 的纯文件系统 probe 不同，node 没有可从文件名读版本的等价物
- 版本归一化：`NormalizeVersion` 剥前导 `v` 后用 toolversion.Parse 校验；注册表内版本一律不带 `v`
- available / install：nodejs.org dist 布局（`index.json` + 每版本目录下 `SHASUMS256.txt` 与归档）
- 版本比较与模糊解析基于 toolversion 公共包（与 maven / gradle 共用）

## 命令组骨架

镜像 maven/gradle 的骨架与契约：自带执行骨架（registry 是 user 级文件，不依赖工作区）、tabwriter 文本、Envelope JSON、install / uninstall 同样的半成品清理与确认契约（TTY 询问 / 非 TTY CONFIRMATION_REQUIRED exit 2 / `--yes` 直通）。差异点：

- registry 为 `~/.barista/node.json`
- **组根本身是执行器**：`barista node -- <args>`（见下"执行器"节与 [executors.md](executors.md)）；管理子命令与透传参数同名时子命令优先；缺 `--` 直接给参数报 USAGE_ERROR（exit 2）；无参数且未给 flag 时显示帮助
- 子命令：list / add / remove / set-default / set-install-dir / which / path / home / install / available / uninstall / use / env（无 discover / config / set-jdk——node 无 JDK 维度）
- 自动命名为 `node-<version>`（完整版本号，不带 `v`），add / install 共用（`NameFor` + `AvailableName` 追加 `-1` / `-2`）；add 与 install 均可 `--name` 覆盖，`--name` 禁止形似版本号
- probe 起子进程，因此 add 比 maven/gradle 的 add 慢；离线测试注入 `execNodeVersion` 桩
- `use` 是 node 组唯一需要工作区的管理命令：写 `<workspace>/.barista/properties.json` 的 `node` 键（跨域工具链声明裸名约定，同 jdk 的 `jdk` 键）；**node 没有 `.java-version` 等价物，不写任何版本文件**

## available

- 数据源与 install 相同（`<DistBase>/index.json`，`DistBase` 可变，测试用 httptest 覆盖）：单请求取全部版本，剥 `v` 前缀、跳过解析不了的条目，`lts` 字段为字符串时记为 LTS 代号（如 `Jod`），按 toolversion.Compare **降序**，最新版本标 `latest`
- 默认视图 `LatestPerMajor(RecentMajors(versions, 6))`（六个最新 major 线各自的最新版本）；`--all` 列全部；installed 标记按版本与注册表比对（不看名字，兼容自定义命名）

## 解析（which / path / home）

- 三命令独立可见（path / home 是 which --pathonly 的快捷方式）
- 解析链：精确名字 → 精确版本（`v` 前缀可省略）→ 数字段前缀逐级扩大（`22.14` → `22`）取 toolversion.Compare 最大者 → NODE_NOT_FOUND
- 不传参数时 which 解析生效默认（workspace properties `node` > node.json `default`）
- 模糊解析只用于只读解析（which / path / home / use / env）；remove / uninstall / set-default 强制精确名字
- set-default 只写 user 级（无 `--scope`）；workspace 级默认用 `barista node use`

## install

- 单 Provider：nodejs.org dist；资产名按平台映射直接构造（`PlatformSuffix`：windows/amd64→`win-x64.zip`、darwin/arm64→`darwin-arm64.tar.gz` 等；不支持报 NODE_UNSUPPORTED_PLATFORM），不从 index.json 的 `files` 数组推导（files 里叫 `osx-*-tar`，文件名实为 `darwin-*`）
- 下载前先经 `Available` + `MatchAvailable` 解析版本：完全一致直接装；否则最长数字前缀取最高者；替换在 CLI 层先告知再调 `InstallResolved`——TTY 下交互确认 `install best match <v>? [Y/n]`（默认 yes，回答 n 中止，结果 skipped / reason=aborted、exit 0），非 TTY / `--json` / `--yes` 不询问、只打 stderr 黄字（JSON detail 带 `requestedVersion`）；完全无匹配报 NODE_NOT_FOUND
- 下载后强制 sha256 校验：下载官方 `<DistBase>/v<ver>/SHASUMS256.txt` 取对应资产行（`ParseSHASums`）；不匹配报 NODE_CHECKSUM_MISMATCH
- 下载镜像（config `download.mirror` / `node.download.mirror` 或 `--mirror` flag 显式覆盖，详见 [mirror.md](mirror.md)）：配置了镜像时把官方资产 URL 的 `DistBase` 前缀替换为镜像基址（`MirrorDownloadURL`）作为主下载源，原官方 URL 作 `InstallResolved` 的 `fallbackURL`（`WithFallback`，回退时 stderr 提示完整官方 URL）；`index.json` 元数据与 `SHASUMS256.txt` 校验和恒从官方取；JSON detail 带 `downloadUrl`（实际使用 URL）与 `mirror`（配置的原始值）
- 解压复用 `download.Extract`（剥归档首层 `node-v<ver>-<suffix>/` 目录，恰好得到安装根）；解压后 probe 校验 `node --version` 与目标版本一致，失败清理目录

## 执行器（node 组根 / npm / npx）

- `barista node [flags] -- <node args>`（组根）与独立根命令 `barista npm` / `barista npx`（cli/npm 薄壳，复用 cli/node 的同一套 `planExec`，仅入口二进制不同）：`--` 后参数原样透传；stdin/stdout/stderr 直接挂父进程
- 解析链（高→低）：`--node <spec>` > 版本文件（`.node-version` > `.nvmrc`）> cwd 命中 repo 的 `properties["node"]` > workspace properties `node` > node.json `default` > ambient（PATH 查找同名二进制）
- 文件检测（`detectNodeVersionFile`）：边界为最近 `.git` 检出根（`workspace.FindGitRoot`，不依赖 repos.json 注册；无检出根时边界为 workspace 根，workspace 外只查 cwd 单点）；`.node-version` 整体优先于 `.nvmrc`（各自按上爬就近取）；内容经 `node.ParseVersionFile` 解析（第一个非空非 `#` 注释行，剥 `v` 前缀）；workspace properties `detect.files=false` 整体关闭（与 mvn / java / gradle 共用开关）
- 显式级别（flag / 文件 / repo / workspace / user default）解析失败响亮报 NODE_NOT_FOUND（exit 2，message 带来源；文件级 message 带文件名）；ambient 落空同样 NODE_NOT_FOUND
- 命中注册安装时：node 直接 exec（原生二进制，Windows 也不经 cmd 包装）；npm/npx 在 Windows 上是 `.cmd` shim，经 `cmd /c` 启动（`ToolBinaryPath` 返回 viaCmd）；ambient 命中的 `.cmd`/`.bat` 同样走 cmd /c
- 子进程环境：命中注册安装时设 `NODE_HOME=<home>` 且 `BinDir(home)` 前置 PATH（npm 脚本内能找到 node 靠这个；`childEnv` 替换已有 NODE_HOME、前置已有 PATH，大小写不敏感）；ambient 不动环境
- node/npm/npx 非零退出 → exit 1；启动失败 → NODE_EXEC_FAILED（exit 2）；`--json` 只影响 exec 前错误与 `--dry-run`；`--dry-run` 的 JSON detail 键：`node` / `args` / `command`，按解析结果可选 `workspace` / `repo`；`node` 来源为文件的层级带 `file` 子键（text 输出同样体现）
- 环境组装是纯函数 `planExec`（cli/node/plan.go，同包单测）；ambient 的 PATH 查找与文件检测在 buildPlan 装配层完成后注入 planInput
- workspace 发现是机会主义的：无 workspace = 跳过 repo / workspace 两级覆盖，仍可命中版本文件与 user 默认

## env

- 仿 jdk env：`--shell sh|cmd|powershell`（别名 bash/zsh→sh、pwsh/ps→powershell），缺省自动检测当前 shell，检测落空回退平台默认（Windows → powershell，其余 → sh）；`--json` 走 Envelope
- 输出 `NODE_HOME` 与 PATH 导出语句；PATH 项的 bin 目录 Windows 为安装根本身（`node.exe` / `npm.cmd` 在根），Unix 为 `<home>/bin`（`BinDir` 辅助函数）

## 偏好分层

- user 级唯一来源是 node.json 的 `default` 字段
- workspace 级覆盖放 `<workspace>/.barista/properties.json` 的 `node` 键（由 `barista node use` 写入）；无 repo 级写入命令，但 per-repo `properties["node"]` 手工声明会被 doctor 校验
- 解析链：workspace properties `node` > node.json `default`（缺省报 NODE_NOT_FOUND）；workspace 覆盖解析不到目标时响亮报错，不静默回退
