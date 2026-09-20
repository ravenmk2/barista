# git 域详细设计

> 全局契约见 [../architecture.md](../architecture.md)；本文档是 git 域（`internal/gitrun` 及 git 命令组的行为规则）的详细设计，改动该域前必读。

## internal/gitrun 包职责

git 域：exec 封装、单仓库操作（含 Clone）、默认分支解析。

## 行为规则

- 默认分支解析链（`internal/gitrun/branch.go`）：repo → top → origin-head → probe(main / master / trunk)，**逐级回退**——候选分支在该仓库无真实 ref 时落到下一级，而不是失败；全落空才报 DEFAULT_BRANCH_UNRESOLVED
- checkout 分支解析：本地已有该分支直接 checkout；远端有同名分支时 `checkout --track origin/<branch>` 建本地跟踪分支；都没有才 `--no-track` 新建（否则 push.default=simple 推不上去）；push 在无 upstream 时自动 `-u origin <branch>`
- status 不静默联网，ahead / behind 基于本地缓存的远端引用
