# Gradle 域详细设计

> 全局契约见 [../architecture.md](../architecture.md)；本文档是 gradle 域（`internal/gradle` + `cli/gradle`）的详细设计，改动该域前必读。

## internal/gradle 包职责

- registry 读写（`~/.barista/gradle.json`，原子写）
- probe：纯文件系统探测——校验 `bin/gradle` / `bin/gradle.bat` 存在、从 `lib/gradle-core-*.jar` 文件名解析版本（剥掉前缀与 `.jar` 的余部必须能解析为版本号，`gradle-core-api-*.jar` 因而不计入；core 缺失时回退 `gradle-launcher-*.jar`；两者俱在则版本必须一致）；不起子进程
- 固有局限：预发行版发行包的 jar 用裸基础版本命名（`gradle-8.11-milestone-1` 内是 `gradle-core-8.11.jar`），不起子进程读不出 qualifier，因此 add / discover 对预发行版 home 只能注册基础版本；install 经版本目录解析目标版本，注册完整版本号（装后 probe 校验对预发行版只比对数字基础段）
- wrapper distributionUrl 版本提取（wrapper.go：ExtractWrapperVersion）
- discover 候选收集：GRADLE_HOME / sdkman / brew / scoop / 平台安装位置 / PATH 反推
- 版本比较与模糊解析基于 toolversion 公共包（与 maven 共用）

## 命令组骨架

镜像 maven 的骨架与契约：自带执行骨架（registry 是 user 级文件，不依赖工作区）、tabwriter 文本、Envelope JSON、install / uninstall 同样的半成品清理与确认契约（TTY 询问 / 非 TTY CONFIRMATION_REQUIRED exit 2 / `--yes` 直通）。差异点：

- registry 为 `~/.barista/gradle.json`（`installations` + `default` + `jdk` + `installDir`）
- 组根本身即执行器：`barista gradle -- <args>`（见 [executors.md](executors.md)）；管理子命令与透传参数同名时子命令优先
- probe 不起子进程，因此 discover 串行不走 runner，也不依赖 java 可用
- 自动命名为 `gradle-<version>`（完整版本号，qualifier 也进名字，如 `gradle-9.0.0-rc-1`），add / discover / install 共用（`NameFor` + `AvailableName` 追加 `-1` / `-2`）；add 与 install 均可 `--name` 覆盖
- `add <path> [--name]` / `install <version> [--name]`：`--name` 禁止形似版本号，避免与模糊解析歧义

## available

- 数据源与 install 相同（services.gradle.org 的 `/versions/all` 全量 JSON，`VersionsBase` 可变，测试用 httptest 覆盖）：单请求取全部版本，过滤 snapshot / nightly / releaseNightly / broken，按 toolversion.Compare 升序，最新 final 标 `latest`
- rc / milestone 预发布（`rcFor` / `milestoneFor` 非空，或 `final` 为 false——远古 `1.0-milestone-*` 条目两字段为空、只能靠 `final` 识别）带 `prerelease` 标记，只在 `--all` 出现
- 默认视图先 `LatestPerMinor` 再 `RecentMajors` 2（两个最新 major 线、每 minor 线最新 final）；`--all` 列全部（含预发布）；installed 标记按版本与注册表比对（不看名字，兼容自定义命名）

## 解析（which / path / home）

- 三命令独立可见
- 解析链：精确名字 → 精确版本 → 数字段前缀逐级扩大（`8.10.1` → `8.10` → `8`）取 toolversion.Compare 最大者 → GRADLE_NOT_FOUND
- 不传参数时 which 解析生效默认（workspace `gradle.default` > gradle.json `default`）
- 模糊解析只用于只读解析；remove / uninstall / set-default 强制精确名字

## install

- 单 Provider：services.gradle.org（`downloadUrl` 直接取自 `/versions/all`，一律 bin zip）
- 下载前先经 `Available` + `MatchAvailable` 解析版本：完全一致直接装；无完全匹配按数字段前缀放宽——最长前缀优先，同一前缀层级内稳定版优先于预发布（rc / milestone），再取最高者（如 `8.10.1` → `8.10.2`、`8` → 最新稳定 8.x、`9.1` 在无稳定 9.1.x 时 → `9.1.0-rc-1`）；解析在 CLI 层完成，替换时先告知再调 `InstallResolved` 下载——TTY 下交互确认 `install best match <v>? [Y/n]`（默认 yes，回答 n 中止，结果 skipped / reason=aborted、exit 0），非 TTY / `--json` / `--yes` 不询问、只打 stderr 黄字（JSON detail 带 `requestedVersion`）；完全无匹配报 GRADLE_NOT_FOUND
- 下载后强制 sha256 校验：优先用 `/versions/all` 内联的 `checksum` 字段，缺省时下载 `checksumUrl` 旁挂文件；不匹配报 GRADLE_CHECKSUM_MISMATCH
- 下载镜像（config `download.mirror` / `gradle.download.mirror` 或 `--mirror` flag 显式覆盖，详见 [mirror.md](mirror.md)）：配置了镜像时把官方 `downloadUrl` 的 `DistributionsBase`（`https://services.gradle.org/distributions`）前缀替换为镜像基址（`MirrorDownloadURL`）作为主下载源，原官方 URL 作 `InstallResolved` 的 `fallbackURL`（`WithFallback`，回退时 stderr 提示完整官方 URL）；`/versions/all` 元数据与 sha256 恒从官方取；JSON detail 带 `downloadUrl`（实际使用 URL，`InstallResult.DownloadURL`）与 `mirror`（配置的原始值）

## config（内省）

`gradle config` 打印合并后的有效配置及来源（source 枚举：workspace / user / repo / builtin / ambient / none），含 cwd 命中的 repo、init script（`.barista/gradle/init.gradle[.kts]`）与 `gradle.user.home` 注入状态。

## 偏好分层

- user 级唯一来源是 gradle.json 的 `default` / `jdk` 字段（`jdk` 由 `barista gradle set-jdk` 写入）
- workspace 级覆盖放 `<workspace>/.barista/properties.json`：`jdk` 是跨域公用属性键（工具链声明，不限 gradle），由 `barista jdk use` 写入；`gradle.default` 由 `barista gradle set-default --scope workspace` 写入
- 解析链按维度分离，不存在统一的 `--flag > workspace properties > gradle.json > 缺省`：Gradle 安装维度没有 flag，链为 `wrapper distributionUrl 版本 > workspace properties gradle.default > gradle.json default`（缺省报 GRADLE_NOT_FOUND）；`--jdk` 只作用 jdk 维度，链为 `--jdk > .java-version 文件 > repo properties["jdk"] > workspace properties jdk > gradle.json jdk > 环境原样`
- workspace 覆盖解析不到目标时响亮报错，不静默回退
- workspace 发现是机会主义的（找不到不算错误，仅意味着无 workspace 级覆盖），home 守卫见 [workspace.md](workspace.md)
