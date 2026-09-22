# upgrade 详细设计

> 全局契约见 [../architecture.md](../architecture.md)；本文档是 upgrade 域（`cli/upgrade` + `internal/upgrade`）的详细设计，改动该域前必读。

`barista upgrade` 是自更新命令。联网纪律：所有联网均为用户显式触发——`upgrade` 与 `jdk available` 查询版本/更新信息，`jdk install` / `jdk download` / `maven install` 下载发行包，git 批量命令经系统 git 访问远端；除此之外永不被动联网。

## internal/upgrade 包职责

release manifest 拉取 / 解析 / 校验（base URL 可注入，默认 GitHub releases/latest/download）、版本比较、sha256 校验、可执行文件自替换。

## 流程

1. 读最新 release 的 `manifest.json` asset（固定 URL，无 API 调用）
2. 比较版本：复用 `maven.CompareVersions`；当前 ≥ 最新幂等报 ok；dev / dirty 构建视为未知，始终可升级
3. 下载平台 asset：复用 download 包（断点续传 + 重试 + TTY 进度条，重试次数同其他下载命令，`--attempts` 可覆盖）
4. sha256 校验，不符报 UPGRADE_CHECKSUM_MISMATCH
5. 替换 `os.Executable()`：
   - Unix：临时文件 rename 原子覆盖
   - Windows：运行中 exe 不可覆盖但可改名——先 rename 为 `.old` 再写入，`.old` 在下次运行 upgrade 时清理（CleanupStale）

## 确认契约与 flag

- TTY 询问（`[Y/n]` 默认 yes，回答 n 中止）/ 非 TTY CONFIRMATION_REQUIRED（exit 2）/ `--yes` 直通
- `--check` 只检测不下载

## 清单生成

- release workflow 调用 `python3 scripts/genmanifest.py` 生成：每个 asset 的 file / sha256 / size 由该脚本计算；workflow 另产出独立的 checksums.txt，不被 manifest 消费
- 发布前经 `barista schema validate manifest` 校验（schema 在 `schemas/manifest.schema.json`）
