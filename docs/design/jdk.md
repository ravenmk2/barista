# JDK 域详细设计

> 全局契约见 [../architecture.md](../architecture.md)；本文档是 jdk 域（`internal/jdk` + `cli/jdk`）的详细设计，改动该域前必读。

## internal/jdk 包职责

- registry 读写（`~/.barista/jdk.json`，原子写）
- probe：起子进程解析 `java -version` / `-XshowSettings`（version / distro）
- 版本解析比较
- `.java-version` 解析与 distro+major 解析（javaversion.go：ParseJavaVersionFile / ResolveJavaVersionSpec）
- discover 候选收集：JAVA_HOME 系 env / sdkman / 平台安装位置 / PATH 反推

## 命令组骨架

- registry 是 user 级文件，命令不依赖工作区，故 cli/jdk 自带执行骨架（不调用 workspace.Load）
- 例外：`use` 是 jdk 组唯一需要工作区的命令（见下）
- output 的 TextRenderer 绑定 git 语义（"repos" 汇总行、git 动作描述），jdk 域文本输出在 cli/jdk 内自行排版（tabwriter）：
  - `which` 例外：恒走 Envelope JSON；`--pathonly` / `path` / `home` 输出纯路径
  - `env` 默认输出 shell 导出语句供 eval 消费：`--shell sh|cmd|powershell`（别名 bash/zsh→sh、pwsh/ps→powershell），缺省自动检测当前 shell（Windows 用 x/sys/windows 读父进程映像名，其次 MSYSTEM/SHELL 标记；Unix 读 `$SHELL`），检测落空回退平台默认；`--json` 走 Envelope
- 色彩复用导出的 `output.Palette`（高亮语义仍集中在 color.go，命令实现不手写 ANSI）；JSON 仍走 output.Envelope

## use

- 写 `<workspace>/.barista/properties.json` 的 `jdk` 键
- cwd 上爬命中 `.git` 检出根时追加原子写 `<检出根>/.java-version`：内容为解析后精确版本，值不同覆盖并提示原值
- `.java-version` 写失败结果降级 failed，不回滚 properties；该追加写不受 `detect.files` 开关影响

## discover

- jdk 域唯一走 runner 并发的命令：每个候选路径一个 Task，probe 起子进程
- 注册在主线程串行进行；命名按 `<distro><major>`，冲突追加 `-1` / `-2`
- 幂等可重复：已注册路径（SamePath 比较）报 skipped，原因放 `Detail.reason`（不带 Error，重复跑不产生错误）；probe 失败的候选同样 skipped 但保留 Error 诊断

## install

- 通过 Provider 接口解析发行版下载地址：
  - temurin → Adoptium API
  - microsoft / corretto → permalink 直链，ArchiveURL 纯拼 URL
  - zulu / graalvm → `embeddedProvider` 读 go:embed 的 `internal/jdk/distros.json`（由 `scripts/gendistros.py` 定期刷新，URL 钉死无有效期、运行期零 API 调用）
- 下载骨架：断点续传（Range 头）+ 指数退避重试（默认 4 次，4xx 不重试）；TTY 下 stderr 渲染进度条（百分比 / 速度 / ETA）
- 实现 `ChecksumProvider` 的 provider（graalvm 嵌入了 sha256 钉值）下载后先校验 sha256，不符报 JDK_CHECKSUM_MISMATCH
- 下载后解压到 `<installDir>/<name>`（剥归档首层目录；download 包统一防 zip-slip / 逃逸 symlink），probe 校验 major 匹配才注册（`managed: true`）；任何失败清理半成品目录

## download（只下载不安装）

- 复用同一 Provider / ChecksumProvider 与下载骨架（进度条 / 重试 / 校验相同），但不读注册表、不解压、不 probe
- `--os` / `--arch` 缺省当前平台、可跨平台（非法值 exit 2）；`--output` 缺省当前目录（值是已存在目录或以路径分隔符结尾按目录拼接文件名，否则视为完整文件路径）
- 文件名经 `download.ResolveFileName`：HEAD 跟随重定向，Content-Disposition 优先、其次最终 URL basename；失败静默回退合成名 `<distro>-jdk-<major>-<os>-<arch>` + 从最终 URL 识别的扩展名（distro 名含 jdk 省略 `-jdk` 段）
- 目标已存在报 JDK_EXISTS；下载写 `<dest>.part` 成功后 rename，`.part` 留存重跑天然续传；sha256 不符删 `.part` 报 JDK_CHECKSUM_MISMATCH

## available

- 经 `Provider.Available` 列出可安装项，数据源随 distro：
  - temurin 实时查 API：available_releases 拿 major 列表与 LTS 标记，每 major 并发查 release_versions 取最新 GA 版本，单点失败只置空 version
  - permalink 两家（microsoft / corretto）返回静态 major 列表
  - zulu / graalvm 读嵌入数据离线返回
- 均按 major 升序，并对照注册表标记 installed

## uninstall

- 仅作用于 managed 条目：删除 `<installDir>/<name>` 整树并注销（级联清 defaults）
- 确认契约：TTY 交互询问；非 TTY 报 CONFIRMATION_REQUIRED（exit 2）；`--yes` 直通
