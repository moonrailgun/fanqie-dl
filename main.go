// SPDX-License-Identifier: GPL-3.0-only
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"fanqie-dl/internal/download"
	"fanqie-dl/internal/fanqie"
)

var version = "dev"

const usage = `fanqie-dl — 番茄小说搜索与全本 TXT 下载

用法：
  fanqie-dl search [--json] [--offset N] [--timeout 20s] 关键词
  fanqie-dl download [-o 文件.txt] [--cache-dir 目录] [--interval 200ms] [--timeout 20s] 书籍ID或链接
  fanqie-dl --version

选项写在关键词或书籍 ID 前。使用 search -h / download -h 查看选项。
下载默认保存为 书籍ID.txt；重复运行会从章节缓存继续，不覆盖已有文件。
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "错误：", err)
		if errors.Is(err, context.Canceled) {
			os.Exit(130)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		_, err := io.WriteString(stdout, usage)
		return err
	}
	if args[0] == "--version" {
		if len(args) != 1 {
			return errors.New("--version does not accept arguments")
		}
		_, err := fmt.Fprintln(stdout, "fanqie-dl", version)
		return err
	}
	if args[0] != "search" && args[0] != "download" {
		return fmt.Errorf("unknown command %q; run fanqie-dl --help", args[0])
	}
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	f.SetOutput(stderr)
	timeout := f.Duration("timeout", 20*time.Second, "每个 HTTP 请求的超时时间")
	if args[0] == "search" {
		asJSON := f.Bool("json", false, "输出 JSON")
		offset := f.Int("offset", 0, "搜索偏移量，下一页使用返回的 offset")
		f.Usage = func() { fmt.Fprintln(stderr, "用法：fanqie-dl search [选项] 关键词"); f.PrintDefaults() }
		if err := f.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if f.NArg() != 1 || strings.TrimSpace(f.Arg(0)) == "" || *offset < 0 || *timeout <= 0 {
			return errors.New("search requires one keyword, offset >= 0 and timeout > 0; flags go before the keyword")
		}
		result, err := fanqie.NewClient(*timeout).Search(ctx, f.Arg(0), *offset)
		if err != nil {
			return err
		}
		if *asJSON {
			return json.NewEncoder(stdout).Encode(result)
		}
		w := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "书籍 ID\t书名\t作者\t分类")
		for _, book := range result.Books {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", book.ID, oneLine(book.Title), oneLine(book.Author), oneLine(book.Category))
		}
		if err := w.Flush(); err != nil {
			return err
		}
		if result.HasMore {
			fmt.Fprintf(stderr, "下一页：fanqie-dl search --offset %d %q\n", result.Offset, f.Arg(0))
		}
		return nil
	}
	output := f.String("o", "", "输出 TXT 路径，默认 书籍ID.txt")
	cache := f.String("cache-dir", "", "章节缓存目录，默认系统缓存目录下的 fanqie-dl")
	interval := f.Duration("interval", 200*time.Millisecond, "批量请求或单章补取之间的最小等待时间")
	f.Usage = func() {
		fmt.Fprintln(stderr, "用法：fanqie-dl download [选项] 书籍ID或链接")
		f.PrintDefaults()
	}
	if err := f.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 1 || *interval < 0 || *timeout <= 0 {
		return errors.New("download requires one book ID/link, interval >= 0 and timeout > 0; flags go before the book ID")
	}
	bookID, err := fanqie.ParseBookID(f.Arg(0))
	if err != nil {
		return err
	}
	if *output == "" {
		*output = bookID + ".txt"
	}
	if *cache == "" {
		root, err := os.UserCacheDir()
		if err != nil {
			return fmt.Errorf("locate cache directory (set --cache-dir): %w", err)
		}
		*cache = filepath.Join(root, "fanqie-dl")
	}
	fmt.Fprintf(stderr, "书籍 %s；章节缓存：%s\n", bookID, filepath.Join(*cache, bookID))
	count, err := download.Run(ctx, fanqie.NewClient(*timeout), bookID, download.Options{
		Output: *output, CacheDir: *cache, Interval: *interval,
		Notice: func(message string) { fmt.Fprintln(stderr, oneLine(message)) },
		Progress: func(done, total int, title string, cached bool) {
			status := "下载"
			if cached {
				status = "缓存"
			}
			fmt.Fprintf(stderr, "[%d/%d] %s %s\n", done, total, status, oneLine(title))
		},
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stderr, "完成：%d 章\n", count)
	_, err = fmt.Fprintln(stdout, *output)
	return err
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
