# fanqie-dl

番茄小说命令行下载器，搜索、目录获取和正文解密逻辑移植自 [ch6vip/fqapp](https://github.com/ch6vip/fqapp)，纯 Go 标准库实现。

## 安装

macOS / Linux：

```sh
curl -fsSL https://raw.githubusercontent.com/moonrailgun/fanqie-dl/main/scripts/install.sh | sh
```

默认安装到 `~/.local/bin`，需确保该目录在 `PATH` 中。

Windows（PowerShell 5.1+）：

```powershell
Invoke-RestMethod https://raw.githubusercontent.com/moonrailgun/fanqie-dl/main/scripts/install.ps1 | Invoke-Expression
```

重复执行即可升级；设置 `FANQIE_DL_INSTALL_DIR` 可自定义安装目录。也可以从 [Releases](https://github.com/moonrailgun/fanqie-dl/releases) 手动下载，或用 Go 1.22+ 构建：`go build -o fanqie-dl .`

## 使用

```sh
# 搜索，返回书籍 ID、书名、作者和分类
fanqie-dl search 三国演义
fanqie-dl search --json --offset 10 三国演义

# 按书籍 ID 或详情页链接下载为 UTF-8 TXT
fanqie-dl download 6838936284967209991
fanqie-dl download -o 三国演义.txt 'https://fanqienovel.com/page/6838936284967209991'

# 只下载第 10–20 章（默认从第一章到最后一章，--end 省略即到最后一章）
fanqie-dl download --start 10 --end 20 6838936284967209991
```

选项需放在关键词或书籍 ID 之前，`download -h` 查看全部选项（`--start`、`--end`、`--cache-dir`、`--interval`、`--timeout`）。指定范围时默认文件名为 `书籍ID_起-止.txt`。

- 章节会缓存，中断后重新执行同一命令即可继续。
- 任一章节失败则不生成文件，也不会覆盖已有文件。
- 插图保存为 `[图片：URL]`。

上游接口非公开 API，可能因服务端变更或限流失效。

## 开发

```sh
go test -race ./...
go vet ./...
bash scripts/release_test.sh && bash scripts/install_test.sh
```

发布：在干净且与远端同步的 `main` 上运行 `./release.sh`（Windows 用 `release.ps1`），推送版本标签后由 GitHub Actions 构建并发布 Release。

## 许可证

GPL-3.0-only，见 [LICENSE](LICENSE)。签名、协议及解密实现改编自 ch6vip 与 fqapp 贡献者的开源代码。
