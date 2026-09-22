# 下载镜像（download mirror）详细设计

> 全局契约见 [../architecture.md](../architecture.md)；本文档是下载镜像机制（跨 jdk / maven / gradle 三域的 install / download 路径）的详细设计，改动相关代码前必读。

## 背景与目标

官方下载源（Adoptium API → GitHub Releases、archive.apache.org、services.gradle.org）在国内网络下慢或不可达。本机制允许为三个域分别配置二进制下载镜像，其余链路不变。

两条贯穿性原则：

- **镜像只替换二进制下载基址，元数据恒走官方**：版本列表（Adoptium API、archive.apache.org 目录、`services.gradle.org/versions/all`）与校验和（maven sha512、gradle sha256、zulu/graalvm 内嵌钉值）的来源不因镜像配置改变。
- **镜像可换字节、不可换信任**：所有校验机制照跑；镜像字节校验失败视为镜像不可信，响亮报错，不静默放过。

## 配置

写在 config.json（user 级 `~/.barista/config.json` 与 workspace 级 `<workspace>/.barista/config.json`，两级合并、workspace 覆盖 user，与 `parallel` / `color` 同一契约）。键均为可选，缺省等于 `official`。

| 键 | 值 | 作用域 |
| --- | --- | --- |
| `download.mirror` | 全局预设名（见下） | 一键给三个域赋默认映射 |
| `jdk.download.mirror` | `official` / 预设名 | jdk install、jdk download |
| `maven.download.mirror` | `official` / 预设名 / 自定义基址 URL | maven install |
| `gradle.download.mirror` | `official` / 预设名 / 自定义基址 URL | gradle install |

优先级（高→低）：下载命令的 `--mirror` flag（显式给出时覆盖一切 config，`--mirror official` 临时禁用镜像）> 分域键 > 全局 `download.mirror` > `official`。flag 值域与对应 config 键一致（jdk 仅预设名，maven / gradle 另接受自定义基址），非法值在参数校验阶段报用法错误（exit 2）。

### 预设

预设名是"每域一个基址"的映射表，不是单一镜像站（没有一家镜像站同时覆盖三个域）：

| 预设 | jdk（仅 temurin） | maven | gradle |
| --- | --- | --- | --- |
| `cn` | TUNA Adoptium | TUNA apache | 腾讯云 |
| `tuna` | TUNA Adoptium | TUNA apache | —（TUNA 无 gradle，设为 official） |
| `huawei` | —（华为无 Temurin，设为 official） | 华为云 apache | 华为云 gradle |
| `tencent` | —（腾讯无 Temurin，设为 official） | 腾讯云 apache | 腾讯云 gradle |

分域键取 `tuna` 之类单站预设时按上表取对应列；该列无覆盖的域保持 `official`。新增预设只需在映射表加行，配置层无感知。

### 自定义基址

仅 maven / gradle 接受自定义 URL（两者镜像都是纯 base 替换，路径模板与官方一致）：

```json
{
  "maven.download.mirror": "https://mirrors.example.com/apache",
  "gradle.download.mirror": "https://mirrors.example.com/gradle"
}
```

- 值为目录基址，不含版本路径段；结尾 `/` 规范化后拼接官方相对路径
- 只接受 `https://`；无法识别为预设名或合法 https 基址时报 `CONFIG_ERROR`（exit 2），不静默回退
- jdk 不接受自定义 URL：Temurin 镜像（TUNA）的 URL 构造规则与官方 API 返回的 GitHub asset URL 完全不同，任意基址无法通用，只开放预设枚举

### 基址映射

| 域 | 官方 | TUNA | 腾讯云 | 华为云 |
| --- | --- | --- | --- | --- |
| jdk/temurin | Adoptium API 返回的 GitHub asset URL | `https://mirrors.tuna.tsinghua.edu.cn/Adoptium` | — | — |
| maven | `https://archive.apache.org/dist` | `https://mirrors.tuna.tsinghua.edu.cn/apache` | `https://mirrors.cloud.tencent.com/apache` | `https://repo.huaweicloud.com/apache` |
| gradle | `https://services.gradle.org/distributions` | — | `https://mirrors.cloud.tencent.com/gradle` | `https://repo.huaweicloud.com/gradle` |

URL 构造细节：

- maven：基址替换后拼 `maven/maven-<major>/<version>/binaries/apache-maven-<version>-bin.<ext>`，路径模板与官方一致
- gradle：基址替换后拼 `gradle-<version>-bin.zip`，与官方一致
- jdk/temurin：版本与文件名仍由 Adoptium API 解析，取 asset 文件名拼 TUNA 目录模板 `<base>/<major>/jdk/<arch>/<os>/<filename>`（arch/os 词元沿用 API 的 `x64`/`aarch64` 等取值，与 TUNA 目录一致）

## 各域生效范围与回退

### jdk

- 仅 temurin 的二进制下载受镜像影响；`jdk available`、版本解析仍走 `api.adoptium.net`
- 镜像 URL 构造：`TemurinMirrorAsset` 先查官方 Adoptium API 的 `/v3/assets/latest/<major>/hotspot`（元数据恒走官方），取首个 asset 的 `binary.package.link` 文件名与 `binary.package.checksum`（sha256），拼 `<base>/<major>/jdk/<arch>/<os>/<filename>`
- **走镜像路径的下载会按 API 返回的 sha256 校验**（官方路径 temurin 无校验，行为不变）；API 未返回 checksum 视为解析失败（镜像字节必须可校验）；asset 解析失败不报错，stderr 提示后直接用官方重定向 URL，降级路径 detail 不带 `mirror`
- microsoft / corretto / zulu / graalvm 无可信镜像，mirror 配置对它们不生效（文档与 `schema show config` 的 description 写明，不报错）
- zulu / graalvm 的内嵌 sha256 钉值与镜像机制无交互，照旧校验

### maven

- install 的 tar.gz/zip 下载走镜像；`maven available` 仍抓官方目录列表；sha512 校验值仍从官方取
- 镜像同步的是 Apache 当前发行树，**历史版本可能缺失**：镜像返回 404 时自动回退官方源重试，text/JSON 输出标注实际下载来源（`detail.downloadUrl`、`detail.mirror`）

### gradle

- install 的 zip 下载走镜像；`/versions/all` 元数据与 sha256（内联或 checksumUrl）仍从官方取
- 发行包全量镜像，一般无缺失；404 回退行为与 maven 一致

### 回退契约（三域统一）

- **可用性失败回退**：镜像 404 / 网络错误 → 自动回退官方源重试一次；`WithFallback` 在开始下载官方源前回调 `Options.OnFallback`，text 模式于此时经 stderr 提示完整官方 URL（`mirror unavailable, falling back to <url>`，TTY 进度条先换行再打印，回退后速率/ETA 重新起算）；JSON detail 恒含 `downloadUrl`（实际下载源，回退后为官方 URL；`jdk download` 沿用既有 `url` 键表达同一语义）
- **完整性失败不回退**：镜像字节校验失败（checksum mismatch）→ 响亮报错（沿用各域 `*_CHECKSUM_MISMATCH`，hint 指向镜像配置：`the bytes came from a mirror; check your mirror configuration`；官方路径报错不变）；不对校验失败做回退，避免掩盖镜像被篡改

## 与其他机制的关系

- 下载层复用 `internal/download`（断点续传 / 重试 / `.part`）；镜像只是在解析 URL 阶段换基址，下载器无感知
- 离线测试沿用 httptest 可变基址（gradle 的 `VersionsBase` 模式）：镜像基址映射表做成数据，测试注入自定义预设或自定义基址即可覆盖
- 不新增命令与 flag；配置通过手工编辑 config.json + `barista schema show config` / `schema validate` 自描述
- `doctor` 不主动探测镜像可达性（不被动联网）；镜像故障由下载时的回退契约兜住

## 改动面清单

- `schemas/config.schema.json`：新增 4 个键及 description（含"元数据恒走官方"说明）
- `internal/workspace`：config 结构加镜像字段（user + workspace 合并）
- `internal/jdk` / `internal/maven` / `internal/gradle`：install/download 的 URL 解析接入镜像映射（预设表 + 自定义基址校验）
- 新增共享的预设映射表（放 `internal/download` 或独立小文件，纯数据 + 解析函数）
- 文档：本文档、architecture.md 索引、commands.md 全局约定的 config 键说明
