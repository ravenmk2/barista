# Maven 域详细设计

> 全局契约见 [../architecture.md](../architecture.md)；本文档是 maven 域（`internal/maven` + `cli/maven`）的详细设计，改动该域前必读。

## internal/maven 包职责

- registry 读写（`~/.barista/maven.json`，原子写）
- probe：纯文件系统探测——校验 `bin/mvn` / `bin/mvn.cmd` 存在、从 `lib/maven-core-*.jar` 文件名解析版本；不起子进程
- 版本比较：数字段 + qualifier token 比较（ComparableVersion 简化版）
- 模糊解析
- wrapper distributionUrl 版本提取（wrapper.go：ExtractWrapperVersion）
- discover 候选收集：MAVEN_HOME / M2_HOME / sdkman / brew / scoop / 平台安装位置 / PATH 反推

## 命令组骨架

镜像 jdk 的骨架与契约：自带执行骨架（registry 是 user 级文件，不依赖工作区）、tabwriter 文本、Envelope JSON、install / uninstall 同样的半成品清理与确认契约（TTY 询问 / 非 TTY CONFIRMATION_REQUIRED exit 2 / `--yes` 直通）。差异点：

- registry 为 `~/.barista/maven.json`（`installations` + `default` + `jdk` + `installDir`）
- probe 不起子进程，因此 discover 串行不走 runner，也不依赖 java 可用
- 自动命名为 `maven-<version>`（完整版本号，qualifier 也进名字，如 `maven-4.0.0-rc-4`），add / discover / install 共用（`NameFor` + `AvailableName` 追加 `-1` / `-2`）；add 与 install 均可 `--name` 覆盖
- `add <path> [--name]` / `install <version> [--name]`：`--name` 禁止形似版本号，避免与模糊解析歧义

## available

- 数据源与 install 相同（Apache archive 目录列表，`ArchiveBase` 可变，测试用 httptest 覆盖）：首页取 `maven-<major>/` 目录，逐 major 并发取版本目录，按 CompareVersions 升序，最新者标 `latest`；单 major 抓取失败只损失该 major
- 默认每个 minor 线只保留最新版本（`LatestPerMinor`），`--all` 列全部；installed 标记按版本与注册表比对（不看名字，兼容自定义命名）

## 解析（which / path / home）

- 三命令独立可见
- 解析链：精确名字 → 精确版本 → 数字段前缀逐级扩大（`3.9.11` → `3.9.*` → `3.*`）取 CompareVersions 最大者 → MAVEN_NOT_FOUND
- 模糊解析只用于只读解析；remove / uninstall / set-default 强制精确名字

## install

- 单 Provider：Apache archive（`archive.apache.org/dist/maven/maven-<major>/<version>/`），一律 tar.gz
- 下载前先经 `Available` + `MatchAvailable` 解析版本：完全一致直接装；无完全匹配按数字段前缀逐级放宽取最高者（如 `3.9.12` → `3.9.16`、`3.9` → 最新 3.9.x、`4.0.0-rc-2` → `4.0.0-rc-6`），替换时 text 模式 stderr 黄字告知、JSON detail 带 `requestedVersion`；完全无匹配报 MAVEN_NOT_FOUND
- 下载后强制 sha512 校验（`.sha512` 旁挂文件），不匹配报 MAVEN_CHECKSUM_MISMATCH

## config（内省）

`maven config` 打印合并后的有效配置及来源（source 枚举：workspace / user / repo / builtin / ambient / none），含 cwd 命中的 repo、settings 与 repo.local 注入状态。

## 偏好分层

- user 级唯一来源是 maven.json 的 `default` / `jdk` 字段（`jdk` 由 `barista maven set-jdk` 写入）
- workspace 级覆盖放 `<workspace>/.barista/properties.json`：`jdk` 是跨域公用属性键（工具链声明，不限 maven），由 `barista jdk use` 写入；`maven.default` 由 `barista maven set-default --scope workspace` 写入
- 解析链：`--flag > workspace properties > maven.json > 缺省`（default 缺省报 MAVEN_NOT_FOUND；jdk 缺省回退环境原样）
- workspace 覆盖解析不到目标时响亮报错，不静默回退
- workspace 发现是机会主义的（找不到不算错误，仅意味着无 workspace 级覆盖），home 守卫见 [workspace.md](workspace.md)
