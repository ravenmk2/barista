# Barista

多仓库工作区的开发环境管理工具，Java 生态优先。

## 构建

```bash
./build.sh             # 交叉编译全平台 → dist/
./build.sh --install   # 只构建当前平台 → ~/.local/bin/
```

要求：Go 1.26+，系统安装 git。

## 使用

创建 `.barista/repos.json`：

```json
{
  "baseUrl": "git@github.com:your-org/",
  "repos": [
    { "name": "order-service", "url": "order-service", "labels": ["java"] }
  ]
}
```

```bash
barista git clone                # 克隆全部（--label/--repo 过滤，可重复取并集）
barista git status               # 所有仓库状态，末尾列出有变更的
barista git checkout <branch>    # 切换分支；没有则从默认分支创建
barista git fetch|pull|push      # 同步（--prune / --rebase / --tags）
```

## 配置

两级 `config.json`（格式相同，均为可选，字段：`parallel`、`color`）：

| 级别      | 路径                                          |
| --------- | --------------------------------------------- |
| user      | `~/.config/barista/config.json`                 |
| workspace | `<workspace>/.barista/config.json`（覆盖 user） |

## Schema

```bash
barista schema list                                # 列出内嵌 JSON Schema
barista schema show repos|config                   # 输出 schema 原文
barista schema validate repos|config [file]        # 校验（config 默认校验 user + workspace 两级）
                      [--scope user|workspace]
```

供 AI Agent 与编辑器在线发现、校验配置。
