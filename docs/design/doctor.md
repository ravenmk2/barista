# doctor 详细设计

> 全局契约见 [../architecture.md](../architecture.md)；本文档是 doctor 域（`cli/doctor`）的详细设计，改动该域前必读。

`barista doctor` 是体检命令：只诊断不修复。每项检查一个 Result：ok / skipped（信息项，带 reason）/ failed（带修复 hint）。

## 执行模型

浅查项装配期同步执行；probe 与 repo 检查走 runner 并发。

## 检查项

始终查 user 级：

- git on PATH
- JAVA_HOME 有效性
- config / jdk / maven / gradle registry 可解析性与引用完整性（defaults / default / jdk / installDir）
- Maven / Gradle 条目版本用纯文件系统 probe 即时比对

机会主义加查 workspace 级（无 workspace 不报错）：

- `.barista` 可加载
- repo 检出存在
- properties 的 `jdk` / `maven.default` / `maven.launch` / `gradle.default` 与 per-repo properties 的 `jdk` / `maven.launch` 可解析（无 per-repo `maven.default` / `gradle.default`）
- `settings.xml` 与 `settings-security.xml` 存在性；`.barista/gradle/` 下 `init.gradle` 与 `init.gradle.kts` 存在性（各自独立报告）
- 逐 repo 浅查 `.java-version`、`maven-wrapper.properties` 与 `gradle-wrapper.properties` 的可解析性（wrapper 检查只看检出根，不上爬）
- repos.json `deps` 引用完整性（dangling 引用报 REPO_NOT_FOUND）

`--deep` 追加：

- JDK 重 probe：起 java 子进程比对注册版本
- repo origin 对清单 URL 的归一化比对

## 输出

- 文本按 scope 分组自排版（不复用绑定 git 语义的 TextRenderer）
- JSON 走 Envelope，detail 含 scope / check
- exit 0 全过 / 1 有 failed / 2 用法错误
