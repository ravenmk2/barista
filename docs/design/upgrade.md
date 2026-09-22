# upgrade 详细设计

> 全局契约见 [../architecture.md](../architecture.md)；本文档是 upgrade 域（`cli/upgrade` + `internal/upgrade`）的详细设计，改动该域前必读。

`barista upgrade` 是自更新命令。联网纪律：所有联网均为用户显式触发——`upgrade` 与 `jdk available` 查询版本/更新信息，`jdk install` / `jdk download` / `maven install` 下载发行包，git 批量命令经系统 git 访问远端；除此之外永不被动联网。

## internal/upgrade 包职责

release manifest 拉取 / 解析 / 校验（base URL 可注入，默认 GitHub releases/latest/download）、版本比较、sha256 校验、归档二进制提取（zip / tar.gz 单条目）、可执行文件自替换。

## 流程

1. 读最新 release 的 `release.json` asset（固定 URL，无 API 调用）
2. 比较版本：复用 `maven.CompareVersions`；当前 ≥ 最新幂等报 ok；dev / dirty 构建视为未知，始终可升级
3. 选择资产：`ExecutableAsset(goos, goarch)`——过滤 `kind` 前缀 `executable-` 且 `platforms` 含当前平台的条目，必须恰好一条；下载 URL 优先取资产的可选 `url`（镜像副本），否则 `<base>/<file>`
4. 下载归档（windows 为 zip，其余为 tar.gz；包内二进制条目为 `barista` / `barista.exe`，另有 LICENSE 与 completions 等附带文件）；复用 download 包（断点续传 + 重试 + TTY 进度条，重试次数同其他下载命令，`--attempts` 可覆盖）
5. sha256 校验归档（`hashes.sha256`），不符报 UPGRADE_CHECKSUM_MISMATCH
6. 按 kind 后缀（`ArchiveFormat`：`executable-zip`→zip、`executable-tgz`→tar.gz）与 `entry` 提取二进制；不认识的后缀报 UPGRADE_EXTRACT_FAILED 并提示客户端太旧、手动安装
7. 替换 `os.Executable()`：
   - Unix：临时文件 rename 原子覆盖
   - Windows：运行中 exe 不可覆盖但可改名——先 rename 为 `.old` 再写入，`.old` 在下次运行 upgrade 时清理（CleanupStale）

## 确认契约与 flag

- TTY 询问（`[Y/n]` 默认 yes，回答 n 中止）/ 非 TTY CONFIRMATION_REQUIRED（exit 2）/ `--yes` 直通
- `--check` 只检测不下载

## 清单（release.json）

职责范围：**一份 release 的自描述**（version + commit + 资产清单），供 `barista upgrade` 消费。不含 release notes、CI 元数据、构建报告内容（与 cargo-dist 的 dist-manifest.json 刻意划界；名字也不复用它，避免格式联想）。

- 顶层：`schemaVersion: 2`、`version`（tag）、`commit`（构建提交的完整 SHA）、`publishedAt`、`assets[]`
- 资产条目：`kind`（角色+容器格式融合词，如 `executable-zip` / `executable-tgz` / `checksums`；schema 用 pattern 不用 enum，新 kind 不破坏旧校验器）、`platforms[]`（`<goos>/<goarch>` 数组，一文件多平台；平台无关资产省略）、`file`、`entry`（executable 的包含二进制路径）、`hashes`（映射，`sha256` 必填，可加算法）、`size`、可选 `url`（绝对下载地址，镜像场景）
- v2 相对 v1 不兼容：旧客户端（抓 `manifest.json`）得到 404 或 schema 版本错误，响亮失败而非装坏文件；v2 内的新增可选字段与新 kind 靠 `additionalProperties: true` + 客户端过滤规则保持向后兼容，schemaVersion bump 只留给破坏性变更
- release workflow 调用 `python3 scripts/gen-release.py dist <version> dist/release.json --commit <sha>` 生成（`--base-url` 可给资产填镜像 URL）；归档命名 `barista-<version>-<goos>-<goarch>.(zip|tar.gz)`，包内二进制统一为 `barista` / `barista.exe`，附带 LICENSE 与 `completions/`（bash/zsh/fish/powershell，由 cobra completion 命令在构建时生成）
- 发布前经 `barista schema validate release` 校验（schema 在 `schemas/release.schema.json`）
