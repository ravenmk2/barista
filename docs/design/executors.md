# 执行器详细设计（mvn / java / gradle / node / npm / npx）

> 全局契约见 [../architecture.md](../architecture.md)；本文档是执行器域（`cli/mvn`、`cli/java`、`cli/gradle` 组根、`cli/node` 组根、`cli/npm` 的 npm/npx 及 maven 启动模式）的详细设计，改动该域前必读。

`barista mvn`、`barista java`、`barista gradle`（组根）、`barista node`（组根）、`barista npm` 与 `barista npx` 是**执行器**而非管理命令：不走 runner / Result 契约，stdin / stdout / stderr 直接挂父进程（流式透传，无 TUI / 渲染层）。

## barista mvn

- 用法：`barista mvn [flags] -- <mvn args>`，`--` 后参数原样透传；缺 `--` 直接给 goal 报 USAGE_ERROR（exit 2）
- 环境组装是纯函数 `planExec`（plan.go，同包单测）
- maven 安装解析链：`wrapper distributionUrl 版本 > workspace maven.default property > maven.json default`
- JDK 解析链：`--jdk > .java-version 文件 > cwd 命中 repo 的 properties["jdk"] > workspace jdk > maven.json jdk > 环境原样`
  - 显式指定的任一级（含文件声明）解析失败响亮报 JDK_NOT_FOUND / MAVEN_NOT_FOUND（exit 2，message 带来源）；命中则只对子进程设 JAVA_HOME
- 文件检测（detectJavaVersionFile / wrapperVersion）在 buildPlan 装配层完成并以 planInput 字段注入，planExec 保持纯函数；wrapper 复用 launch.go 的 findBasedir 定位 `.mvn/wrapper/maven-wrapper.properties`；查找边界为最近 `.git` 检出根（`workspace.FindGitRoot`，不依赖 repos.json 注册）；workspace properties 键 `detect.files=false` 整体关闭
- 执行：Unix 直接 `bin/mvn`；Windows 经 `cmd /c bin/mvn.cmd`
- mvn 非零退出 → exit 1；barista 自身配置错误 → exit 2 + 机器可读码
- `--json` 只影响 exec 前的错误（ErrorEnvelope）与 `--dry-run` 输出；exec 后输出归属 mvn
- workspace 发现是机会主义的：无 workspace = 跳过 repo / workspace 两级覆盖，纯 user 级默认透传

### 注入规则

- `.barista/maven/settings.xml` 存在即注入 `-s`（零配置私有 settings）；同目录 `settings-security.xml` 注入 `-Dsettings.security`
- workspace property `maven.repo.local` 注入 `-Dmaven.repo.local`（相对路径锚定 workspace root）；CLI 注入优先级高于 settings.xml 的 `<localRepository>`，两者同设时 property 赢
- **透传参数已含 `-s` / `--settings` / 对应 `-D` 时跳过该注入**（用户显式优先）

## barista java

同型执行器（镜像 cli/mvn 骨架）：`barista java [--jdk spec] [--dry-run] -- <java args>`。

- JDK 解析链：`--jdk > .java-version 文件 > repo properties["jdk"] > workspace jdk > ambient（PATH 查找 java）`——无 maven.json jdk 一级
- 文件检测与边界语义同 cli/mvn（detectJavaVersionFile，detect.files 开关共用）
- 显式级别失败响亮报 JDK_NOT_FOUND（exit 2）；ambient 也落空同样 JDK_NOT_FOUND
- 命中注册 JDK 时直接 exec 其 `bin/java`（Windows `bin/java.exe`，不经 cmd 包装），只对子进程设 JAVA_HOME
- java 非零退出 → exit 1；启动失败 → JAVA_EXEC_FAILED（exit 2）
- 环境组装同为纯函数 planExec：ambient 的 PATH 查找在 buildPlan 完成、以 ambientBin 注入，保持 planExec 可测

## barista gradle（组根执行器）

gradle 命令组的组根本身是执行器：`barista gradle [flags] -- <gradle args>`，`--` 后参数原样透传；管理子命令与透传参数同名时子命令优先（如 `barista gradle install 8.10.2` 走 install 子命令而非执行 `gradle install`）；缺 `--` 直接给 task 报 USAGE_ERROR（exit 2）；无参数且未给 flag 时显示帮助。

- 环境组装是纯函数 `planExec`（cli/gradle/plan.go，同包单测）
- Gradle 安装解析链：`wrapper distributionUrl 版本 > workspace gradle.default property > gradle.json default`（无 repo 级）
- JDK 解析链：`--jdk > .java-version 文件 > cwd 命中 repo 的 properties["jdk"] > workspace jdk > gradle.json jdk > 环境原样`
  - 显式指定的任一级（含文件声明）解析失败响亮报 JDK_NOT_FOUND / GRADLE_NOT_FOUND（exit 2，message 带来源）；命中则只对子进程设 JAVA_HOME
- 文件检测（detectJavaVersionFile / wrapperVersion）在 buildPlan 装配层完成并以 planInput 字段注入，planExec 保持纯函数；wrapper 从 cwd 上爬定位 `gradle/wrapper/gradle-wrapper.properties`，查找边界为最近 `.git` 检出根（`workspace.FindGitRoot`，无 `MAVEN_BASEDIR` 对应物）；workspace properties 键 `detect.files=false` 整体关闭（与 mvn / java 共用开关）
- 只有 script 启动：Unix 直接 `bin/gradle`；Windows 经 `cmd /c bin/gradle.bat`；无 `--launch` 对应物
- gradle 非零退出 → exit 1；启动失败 → GRADLE_EXEC_FAILED（exit 2）；barista 自身配置错误 → exit 2 + 机器可读码
- `--json` 只影响 exec 前的错误（ErrorEnvelope）与 `--dry-run` 输出；`--dry-run` 的 JSON detail 键：`gradle` / `jdk` / `args` / `command`，按解析结果可选 `workspace` / `repo` / `initScripts` / `gradleUserHome`；`gradle` / `jdk` 来源为文件的层级带 `file` 子键（text 输出同样体现）
- workspace 发现是机会主义的：无 workspace = 跳过 repo / workspace 两级覆盖与全部注入，纯 user 级默认透传

### 注入规则

- `.barista/gradle/init.gradle` 与 `.barista/gradle/init.gradle.kts` 存在即各注入一个 `-I`（零配置 init script）；**透传参数已含 `-I` / `--init-script` 时整体跳过**（用户显式优先）
- workspace property `gradle.user.home` 注入 `--gradle-user-home`（相对路径锚定 workspace root，支持 `~` 展开）；**透传已含 `-g` / `--gradle-user-home` / `-Dgradle.user.home` 时跳过**

## barista node（组根执行器）、barista npm、barista npx

node 命令组的组根本身是执行器（同 gradle 的子命令优先契约）；`barista npm` / `barista npx` 是独立根命令（cli/npm 薄壳，无管理子命令，照 cli/java 形态），三者共用 cli/node 的同一套纯函数 `planExec`（planInput 带 `tool` 字段），仅入口二进制不同。

- 用法：`barista node|npm|npx [flags] -- <args>`，`--` 后参数原样透传；缺 `--` 直接给参数报 USAGE_ERROR（exit 2）；node 组根无参数且未给 flag 时显示帮助（npm/npx 无此分支，同 java）
- Node 安装解析链：`--node > 版本文件（.node-version > .nvmrc）> repo properties["node"] > workspace node > node.json default > ambient（PATH 查找同名二进制）`
  - 显式指定的任一级（含文件声明）解析失败响亮报 NODE_NOT_FOUND（exit 2，message 带来源；文件级带文件名）；ambient 落空同样 NODE_NOT_FOUND
- 文件检测在 buildPlan 装配层完成并以 planInput 字段注入；查找边界为最近 `.git` 检出根（`workspace.FindGitRoot`，不依赖 repos.json 注册）；`.node-version` 整体优先于 `.nvmrc`，各自上爬就近；内容取第一个非空非注释行、剥 `v` 前缀（`node.ParseVersionFile`）；workspace properties 键 `detect.files=false` 整体关闭（与 mvn / java / gradle 共用）
- 命中注册安装时：node 直接 exec（原生二进制，Windows 不经 cmd 包装）；npm/npx 在 Windows 经 `cmd /c` 启动 `<home>/npm.cmd` / `npx.cmd`（.cmd shim 必须走 cmd，同 gradle.bat 先例）；ambient 命中的 .cmd/.bat 同样 cmd /c
- 子进程环境注入 `NODE_HOME=<home>` 且安装 bin 目录前置 PATH（Windows 为安装根，Unix 为 `<home>/bin`）；ambient 不动环境
- 子进程非零退出 → exit 1；启动失败 → NODE_EXEC_FAILED（exit 2）
- `--json` 只影响 exec 前的错误（ErrorEnvelope）与 `--dry-run` 输出；`--dry-run` 的 JSON detail 键：`node` / `args` / `command`，按解析结果可选 `workspace` / `repo`；`node` 来源为文件的层级带 `file` 子键（text 输出同样体现）

## mvn 启动模式（launch.go）

- 解析链：`--launch > repo properties["maven.launch"] > workspace properties["maven.launch"] > 默认 script`；非法值报 CONFIG_ERROR
- `script` 即上述包装脚本路径（Unix `bin/mvn` / Windows `cmd /c bin/mvn.cmd`）
- `java` 绕过包装脚本直启 java：消除 `cmd /c` 二次解析对 `%` / `&` / `|` 参数的变形风险，双平台同一代码路径；复刻 mvn 脚本契约（3.6–3.9 实测同构）：
  - java 取 JDK 路径的 `bin/java`（ambient 时 PATH 查找）
  - glob `boot/plexus-classworlds-*.jar` 恰一个 + `bin/m2.conf` 存在，否则 MAVEN_EXEC_FAILED（hint 回退 `--launch script`）
  - basedir 上爬 `.mvn`（感知 `-f` / `--file`；`MAVEN_BASEDIR` env 优先，落空回 cwd）
  - 读 `.mvn/jvm.config`；透传 `MAVEN_OPTS` / `MAVEN_DEBUG_OPTS`；`MAVEN_ARGS` 仅 ≥3.9 追加
  - `-Dlibrary.jansi.path` 在 `lib/jansi-native` 存在时设置
- 差异声明：java 模式不执行 `mavenrc_pre/post` 钩子
