# fanqie-dl

从 [ch6vip/fqapp](https://github.com/ch6vip/fqapp) 提取小说搜索、目录获取和正文解密，用 Go 实现的独立命令行下载器。只使用 Go 标准库，无需 Flutter、Rust、Python、设备注册服务或第三方解析服务。

## 构建与使用

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

# 可选：请求真实上游，验证搜索、目录、单章及批量正文解密
FANQIE_LIVE_TEST=1 go test ./internal/fanqie -run TestLive -v
```

离线测试覆盖签名参考向量、POST 请求体签名、DH/CM 解密、批量对象/数组响应、部分失败与单章补取、100 章分批、目录顺序和缺章检查、HTML 转文本、失败恢复、缓存损坏和已有文件保护。

2026-10-18 在 macOS arm64 / Go 1.22.5 实测：《三国演义》`6838936284967209991` 的 124 个目录条目（含 120 回正文）全部下载完成。主批次为 100 + 24 章；其中一章为纯插图，批量响应仅含标题，单独重试两次后由单章接口补回图片链接，另查询一次首章补齐作者。逐章核对正文与目录顺序后，复用全部 124 个缓存章节再次导出，两个文件逐字节一致；复用时没有正文请求。这是该书、该次接口状态的验证，不代表所有书籍都能下载。

## 许可证

本项目按 GPL-3.0-only 发布，完整条款见 [LICENSE](LICENSE)。签名、协议及解密实现改编自 ch6vip 与 fqapp 贡献者的开源代码；源文件中保留来源标记。
