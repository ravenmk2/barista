# deps 域详细设计

> 全局契约见 [../architecture.md](../architecture.md)；本文档是 deps 域（`internal/deps` + `cli/deps`）的详细设计，改动该域前必读。

## internal/deps 包职责

从 `repos[].deps` 声明建图；dangling 检测、SCC 环检测（Tarjan）、构建层级（1 + 依赖链最长路径，环成员共享层级）。纯函数，不碰文件系统与子进程。

## 命令组（cli/deps）

**纯只读**的清单消费者：`repos[].deps` 声明（repo 名数组，人维护的稳定事实）经 `internal/deps` 建图。

- `show`：输出邻接表，dangling 引用与环如实标注
- `order`：输出构建层级序——层级 = 1 + 依赖链最长路径，同层可并行构建，环成员折叠同层并标注
- 图在完整清单上构建，选择器（`--repo` / `--label`，语义同 git 组）只过滤输出
- 不克隆也可运行（不触碰检出）
- 无失败路径：环与 dangling 是事实展示而非失败（exit 0），仅用法 / 配置错误 exit 2
- 文本自排版（tabwriter，骨架镜像 repo 组）；JSON 走 Envelope：results 按声明顺序，order 的层级放 `detail.buildLevel`
