# fanqie-dl

从 [ch6vip/fqapp](https://github.com/ch6vip/fqapp) 提取小说搜索、目录获取和正文解密，用 Go 实现的独立命令行下载器。只使用 Go 标准库，无需 Flutter、Rust、Python、设备注册服务或第三方解析服务。

## 安装与使用

macOS / Linux 可以使用安装脚本（仓库发布首个 Release 后可用）：

```sh
curl -fsSL https://raw.githubusercontent.com/moonrailgun/fanqie-dl/main/scripts/install.sh | sh
fanqie-dl --version
fanqie-dl search 三国演义
```

脚本自动识别系统和 amd64 / arm64 架构，下载最新正式 Release，验证 SHA-256 后安装到 `~/.local/bin/fanqie-dl`，无需 `sudo` 或 Go 环境。重复执行即可升级；下载或校验失败会保留旧版本。脚本不会修改 shell 配置，如果该目录尚未加入 `PATH`，在 `~/.zshrc`（Bash 用户使用 `~/.bashrc`）中添加：

```sh
export PATH="$HOME/.local/bin:$PATH"
```

重新打开终端后生效。已有源码时也可以运行 `sh scripts/install.sh`；设置 `FANQIE_DL_INSTALL_DIR` 可指定其他安装目录。

Windows 在 PowerShell 5.1 或更新版本中运行（同样需要先发布首个 Release）：

```powershell
Invoke-RestMethod https://raw.githubusercontent.com/moonrailgun/fanqie-dl/main/scripts/install.ps1 | Invoke-Expression
fanqie-dl --version
```

自动选择 Windows amd64 / arm64 ZIP 包并校验 SHA-256，安装到 `%LOCALAPPDATA%\Programs\fanqie-dl\fanqie-dl.exe`，无需管理员权限或 Git。安装目录会加入用户 `PATH`，当前 PowerShell 立即生效，其他终端需重新打开。重复执行即可升级；下载、校验或替换失败会保留旧文件，升级前请退出正在运行的 `fanqie-dl`。设置 `$env:FANQIE_DL_INSTALL_DIR` 可自定义目录；已有源码时可执行 `powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\install.ps1`，这不会修改系统执行策略。

可以从 [GitHub Releases](https://github.com/moonrailgun/fanqie-dl/releases) 下载对应系统和架构的压缩包，解压后直接运行；Windows 使用 `fanqie-dl.exe`。每个版本附带 `checksums.txt`（SHA-256）。

从源码构建：

需要 Go 1.22 或更新版本：

```sh
go build -o fanqie-dl .

# 搜索，结果包含书籍 ID、书名、作者和分类
./fanqie-dl search 三国演义

# 下载当前目录内的全部章节，按目录顺序保存为 UTF-8 TXT
./fanqie-dl download -o 三国演义.txt 6838936284967209991

# 也接受番茄书籍详情页链接
./fanqie-dl download 'https://fanqienovel.com/page/6838936284967209991'

# JSON 输出；继续搜索时使用上次返回的 offset
./fanqie-dl search --json 三国演义
./fanqie-dl search --offset 10 三国演义

./fanqie-dl download -h
```

**选项放在关键词或书籍 ID 前。** 多个词组成的关键词用引号包起来。`/reader/章节ID` 不是书籍链接。

## 下载行为

- 获取完整目录，校验章节列表一致性，再使用原项目的 `full/mget/v1` 接口，每批最多请求 100 章，通过 CM 密钥交换和 AES 解密后逐章缓存。响应中的书籍与章节 ID 必须匹配；空正文、仅有章名或解密失败的内容不会写入缓存。
- 连载作品导出本次开始下载时目录中的全部章节。不会等待未来更新。
- 每批最多尝试 3 次，部分成功时保存有效章节，后续只重试缺失部分。仍未成功的章节使用原单章 DH 接口补取，最多尝试 3 次；批量请求及单章补取会明确打印到 stderr。
- 默认请求间隔 200ms；重试时额外等待 1 秒、2 秒。可用 `--interval 500ms` 调慢、`--timeout 30s` 调整单次请求超时。
- 批量接口未返回作者时，额外查询一次首章来补齐作者并保存到缓存；这次元信息查询失败不影响已完整取得的正文。
- 进度写到 stderr，成功后的文件路径写到 stdout。任何一章失败都会退出并返回非零状态，不生成缺章的最终 TXT。Ctrl+C 返回 130。
- 章节缓存默认位于系统缓存目录下的 `fanqie-dl/书籍ID/`，可用 `--cache-dir ./cache` 指定。中断后原样重跑即可继续；目录仍会重新获取，已缓存章节会复用。缓存不自动检查正文修订，需要重抓时使用新的缓存目录。
- 默认输出 `书籍ID.txt`。已有文件会被拒绝覆盖；重新导出请用 `-o` 指定新文件名。输出目录会自动创建。
- TXT 保留段落和章节标题；插图保存为 `[图片：URL]`，不下载图片文件。

输出在同一目录先写临时文件，全部成功后通过硬链接原子发布，避免覆盖下载期间出现的同名文件。输出目录的文件系统需支持硬链接（常见 APFS、ext4、NTFS 支持）；不支持时会报错，章节缓存仍然保留。

## 提取范围与结构

参考版本固定为 [`e43762d0ba9b192b5e8cb7db24a5b6e06f2216e3`](https://github.com/ch6vip/fqapp/tree/e43762d0ba9b192b5e8cb7db24a5b6e06f2216e3)。原项目由 Flutter UI、Rust 接口层、签名和多种正文解密通道组成；本项目保留搜索、批量正文及单章补取路径。

| 原项目 | 本项目 | 用途 |
| --- | --- | --- |
| `rust/src/endpoints/search.rs` | `internal/fanqie/client.go` | 搜索及偏移量分页 |
| `rust/src/endpoints/book.rs` | `internal/fanqie/client.go` | 网站完整目录及章节顺序 |
| `rust/src/endpoints/full.rs`、`upstream/cm.rs` | `internal/fanqie/batch.go`、`crypto.go` | 批量 POST、CM 密钥交换、AES-256-CBC 解密 |
| `rust/src/endpoints/dhcontent.rs`、`upstream/dh.rs` | `internal/fanqie/client.go`、`crypto.go` | 单章 DH 解密及补取 |
| `rust/src/sign/{argus,ladon,sm3,simon,protobuf}.rs` | `internal/fanqie/sign.go` | 请求签名 |
| `lib/services/chapter_download.dart`、`book_txt_export.dart` | `internal/download/download.go` | 全本下载、章节缓存及 TXT 合并 |
| Flutter 操作入口 | `main.go` | `search` / `download` 命令 |

接口调用链：

```text
search   → 搜索接口 → 书籍 ID
download → 完整目录 → 筛选未缓存章节 → 批量签名 POST → CM/AES 解密
         → 缺失部分重试 / 单章 DH 补取 → 章节缓存 → 按目录合并完整 TXT
```

未移植书架、阅读器、听书、漫画、EPUB、设备池及其他正文备用通道。接口未返回可用正文时会报错，不跳过章节。上游接口并非稳定公开 API，服务端变更或限流可能使搜索或下载失效。

## 验证

```sh
go test ./...
go test -race ./...
go vet ./...
bash scripts/release_test.sh
bash scripts/install_test.sh

# 可选：请求真实上游，验证搜索、目录、单章及批量正文解密
FANQIE_LIVE_TEST=1 go test ./internal/fanqie -run TestLive -v
```

Windows 脚本测试（PowerShell 5.1 或 7，需要 Git for Windows）：

```powershell
./scripts/install_test.ps1
./scripts/release_test.ps1
```

离线测试覆盖签名参考向量、POST 请求体签名、DH/CM 解密、批量对象/数组响应、部分失败与单章补取、100 章分批、目录顺序和缺章检查、HTML 转文本、失败恢复、缓存损坏和已有文件保护。

2026-10-18 在 macOS arm64 / Go 1.22.5 实测：《三国演义》`6838936284967209991` 的 124 个目录条目（含 120 回正文）全部下载完成。主批次为 100 + 24 章；其中一章为纯插图，批量响应仅含标题，单独重试两次后由单章接口补回图片链接，另查询一次首章补齐作者。逐章核对正文与目录顺序后，复用全部 124 个缓存章节再次导出，两个文件逐字节一致；复用时没有正文请求。这是该书、该次接口状态的验证，不代表所有书籍都能下载。

## 发布版本

先将改动提交并推送到 `main`，然后执行（需要 Git 和仓库推送权限）：

```sh
./release.sh            # 交互选择 patch / minor / major，或直接输入目标版本
./release.sh --dry-run  # 交互选择并预览，不创建或推送发布标签
./release.sh patch      # 例如 v0.1.0 -> v0.1.1
./release.sh minor      # 例如 v0.1.1 -> v0.2.0
./release.sh major      # 例如 v0.2.0 -> v1.0.0
./release.sh 1.0.0      # 指定目标版本，也支持 v1.0.0
```

Windows 安装完整的 [Git for Windows](https://gitforwindows.org/) 后，可以在 Git Bash 中执行上述命令，或在 PowerShell 中执行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\release.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\release.ps1 --dry-run
powershell -NoProfile -ExecutionPolicy Bypass -File .\release.ps1 minor
powershell -NoProfile -ExecutionPolicy Bypass -File .\release.ps1 1.0.0
```

PowerShell 入口调用 Git 自带的 Bash，复用相同的版本选择、检查与发布逻辑，无需 WSL；使用 PowerShell 7 时可以将命令中的 `powershell` 替换为 `pwsh`。

未指定升级类型或目标版本时，先显示当前版本及各选项对应的新版本，输入 `1` / `2` / `3`、对应类型名称，或直接输入 `1.0.0` / `v1.0.0` 这样的目标版本，选定后显示版本变更。目标版本必须符合 `X.Y.Z` 格式（数字不含前导零，可加 `v` 前缀）且高于当前版本；空输入或无效输入会重新询问，输入结束则取消发布。自动化调用请显式传入升级类型或目标版本。

版本号来自 `vMAJOR.MINOR.PATCH` Git 标签，不维护额外版本文件；没有历史版本时，三种升级选项默认发布 `v0.1.0`，也可直接指定其他目标版本。发布包的 `fanqie-dl --version` 显示该版本；普通 `go build` 显示 `dev`。

脚本会拉取远端 `main` 和标签，要求工作区干净、当前分支为 `main` 且与 `origin/main` 一致，当前提交也不能已有发布标签。通过检查后创建带注释的标签并只推送该标签，不代为提交代码或推送分支。`--dry-run` 同样执行这些检查和拉取操作。

标签推送触发 `.github/workflows/release.yml`：先在 Windows 上用 PowerShell 5.1 和 7 验证安装、发布脚本，再运行离线测试与 `go vet`，通过后用 GoReleaser 构建 macOS、Linux、Windows 的 amd64/arm64 共 6 个版本，发布 GitHub Release、压缩包（Windows 为 ZIP，其余为 tar.gz）和校验文件。工作流使用仓库自带的 `GITHUB_TOKEN`，无需额外配置发布密钥；仓库需启用 GitHub Actions。

推送成功只表示已触发发布，构建和上传结果需在仓库 Actions 中查看。标签推送失败时，脚本保留本地标签并打印重试命令；Actions 失败时修复原因并重新运行对应工作流，代码修复则提交后发布新版本。

## 许可证

本项目按 GPL-3.0-only 发布，完整条款见 [LICENSE](LICENSE)。签名、协议及解密实现改编自 ch6vip 与 fqapp 贡献者的开源代码；源文件中保留来源标记。
