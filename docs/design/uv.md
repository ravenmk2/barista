# uv 详细设计

> 全局契约见 [../architecture.md](../architecture.md)；本文档是 uv 域（`cli/uv` + `internal/uv`）的详细设计，改动该域前必读。

`barista uv install` 是轻量安装器：把 uv（Astral 的 Python 包/项目管理器，单二进制）装进 `~/.local/bin`，主要服务 Agent 跑 python 临时脚本的场景。**Python 版本管理不进入 barista**，全部委托给 uv 自身（`uv python install` / PEP 723 `uv run`）；因此 uv 不设注册表、无 per-repo 钉住，与 jdk/maven/gradle 域刻意不同构。

联网纪律同全局契约：仅用户显式触发 `uv install` 时联网。

## 发布资产契约（astral-sh/uv）

- 资产命名 `uv-<rust-target>.(zip|tar.gz)`，六平台映射：windows→`{x86_64,aarch64}-pc-windows-msvc.zip`，linux→`{x86_64,aarch64}-unknown-linux-gnu.tar.gz`，darwin→`{x86_64,aarch64}-apple-darwin.tar.gz`；不支持的 GOOS/GOARCH 报 UV_UNSUPPORTED_PLATFORM
- 每个资产带同名 `.sha256` 旁挂校验文件
- 下载源由 `--source` 指定：`astral`（默认，Astral 官方 CDN `releases.astral.sh`，无 latest 端点）/ `github`（`github.com/astral-sh/uv/releases`，有 `/latest/download/` 固定 URL）/ 自定义 `https://` base（必须显式 `--version`）。两个命名源均为 Astral 官方，字节一致
- latest 解析：github 源用固定 URL 不解析；astral 源拉 cargo-dist 安装脚本 `releases.astral.sh/installers/uv/latest/uv-installer.sh` 解析硬编码的 `APP_VERSION`——因此 astral 源在下载前就知道具体版本，已装同版本时直接 skipped 不下载
- 压缩包根目录含可执行文件 `uv` / `uvx`（windows 另有 `uvw`），全部安装——注意 `download.Extract` 的 stripFirst 语义不适用（根目录文件会被剥掉），本域自带解压

## 流程

1. 平台映射；`exec.LookPath("uv")` 命中**且不是目标路径**时提示已可用（带探测到的版本），TTY 询问是否继续（默认 no），非 TTY 报 CONFIRMATION_REQUIRED（exit 2），`--yes` 直通
2. 探测目标路径 `~/.local/bin/uv` 的已装版本（best-effort）
3. 解析目标版本：`--version` 给定则用之；astral 源解析 latest（installer 脚本的 `APP_VERSION`）；github 源走 latest 固定 URL 不解析
4. 已装版本等于目标版本 → skipped（幂等，不下载）
5. 下载归档 + `.sha256`（复用 download 包：断点续传/重试/`--attempts`/TTY 进度条），校验不符报 UV_CHECKSUM_MISMATCH
6. 解压可执行文件到临时目录并探测新版本（github latest 路径在此与已装版本比对，相同则 skipped）
7. 复制进 `~/.local/bin`（同目录 .tmp + rename）；装完检查 `~/.local/bin` 是否在 PATH，不在则在 stderr 给出分平台的配置提示（不自动改 rc 文件）
8. 成功输出 hint `uv python install 3.14` 引导下一步

## 错误码与 doctor

- 新错误码：UV_UNSUPPORTED_PLATFORM / UV_DOWNLOAD_FAILED / UV_CHECKSUM_MISMATCH / UV_INSTALL_FAILED
- doctor 检查 `uvAvailable`：PATH 命中报 ok（带路径）；仅 `~/.local/bin` 存在报 skipped（not on PATH）；完全没有报 skipped（hint: `barista uv install`）——uv 是可选工具，缺失不构成 failed
