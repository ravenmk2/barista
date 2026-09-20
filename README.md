# Barista

[![Go version](https://img.shields.io/github/go-mod/go-version/ravenmk2/barista)](go.mod)
[![License](https://img.shields.io/github/license/ravenmk2/barista)](LICENSE)
[![Test](https://github.com/ravenmk2/barista/actions/workflows/test.yml/badge.svg)](https://github.com/ravenmk2/barista/actions/workflows/test.yml)
[![Release](https://img.shields.io/github/v/release/ravenmk2/barista)](https://github.com/ravenmk2/barista/releases)

多仓库工作区的开发环境管理工具，Java 生态优先。输出面向脚本与 AI Agent：结构化 JSON、机器可读错误码、非交互环境永不阻塞、操作幂等可重复。

- **JDK / Maven 工具链管理**：发现、注册、一键安装（Temurin / Microsoft JDK / Corretto / Zulu / GraalVM）、多版本默认切换
- **工作区感知的 mvn / java**：按 repo / 工作区 / 用户多级自动选用 JDK，注入私有 settings 与本地仓库
- **环境体检与自更新**：`barista doctor` 诊断配置与检出，`barista upgrade` 一键升级
- **动态 shell 补全**：bash / zsh / fish / PowerShell，仓库名、JDK、Maven 等参数智能候选

## 安装

从 [Releases](https://github.com/ravenmk2/barista/releases) 下载对应平台的二进制，装好后可用 `barista upgrade` 自更新。

或从源码构建（要求 Go 1.26+ 与 git）：

```bash
./build.sh --install   # 构建并装到 ~/.local/bin/
```

## 快速上手

在工作区根目录初始化（已有检出的目录可加 `--scan` 直接导入）：

```bash
barista init --repo-base-url git@github.com:your-org/ --scan
```

手工编辑的 `.barista/repos.json` 形如：

```json
{
  "baseUrl": "git@github.com:your-org/",
  "repos": [
    { "name": "order-service", "url": "order-service", "labels": ["java"] }
  ]
}
```

```bash
barista git clone              # 克隆全部仓库（--label/--repo 过滤）
barista git status             # 所有仓库状态一览

barista jdk available          # 看看有哪些 JDK 可装
barista jdk install temurin17  # 一键安装
barista maven install 3.9.9    # 安装 Maven

barista jdk use 17             # 本工作区用 JDK 17
barista mvn -- clean install   # 自动带上工作区的 JDK 与 settings
barista doctor                 # 环境体检
```

配置文件全部自描述，可用 `barista schema` 查看与校验。

## 文档

完整命令与配置参考：[docs/commands.md](docs/commands.md)

## License

[Apache-2.0](LICENSE)
