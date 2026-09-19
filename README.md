# Barista

[![Go version](https://img.shields.io/github/go-mod/go-version/ravenmk2/barista)](go.mod)
[![License](https://img.shields.io/github/license/ravenmk2/barista)](LICENSE)
[![Test](https://github.com/ravenmk2/barista/actions/workflows/test.yml/badge.svg)](https://github.com/ravenmk2/barista/actions/workflows/test.yml)
[![Release](https://img.shields.io/github/v/release/ravenmk2/barista)](https://github.com/ravenmk2/barista/releases)

多仓库工作区的开发环境管理工具，Java 生态优先。

- **多仓库 git 批量操作**：clone / status / checkout / fetch / pull / push，label 过滤，并发执行
- **JDK / Maven 工具链管理**：发现、注册、一键安装、多版本默认切换
- **工作区感知的 mvn**：按 repo / 工作区 / 用户三级自动选用 JDK，注入私有 settings 与本地仓库

## 安装

```bash
./build.sh --install   # 构建并装到 ~/.local/bin/
```

要求：Go 1.26+，系统安装 git。

## 快速上手

在工作区根目录初始化（已有检出的目录可加 `--scan` 直接导入）：

```bash
barista init --base-url git@github.com:your-org/ --scan
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

barista jdk discover           # 发现本机 JDK 并注册
barista jdk install temurin17  # 或一键安装
barista maven install 3.9.9    # 安装 Maven（sha512 校验）

barista jdk use 17             # 本工作区用 JDK 17
barista mvn -- clean install   # 自动带上工作区的 JDK 与 settings
```

配置文件全部自描述（内嵌 JSON Schema，可用 `barista schema` 发现与校验）。完整命令与配置参考：[docs/commands.md](docs/commands.md)。
