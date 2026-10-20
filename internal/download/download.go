// SPDX-License-Identifier: GPL-3.0-only
package download

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fanqie-dl/internal/fanqie"
)

type Source interface {
	Directory(context.Context, string) ([]fanqie.Chapter, error)
	Chapter(context.Context, string, string) (fanqie.Content, error)
	Chapters(context.Context, string, []string) (map[string]fanqie.Content, error)
}

type Options struct {
	Output, CacheDir string
	Interval         time.Duration
	// Start and End select 1-based inclusive chapters; 0 means first/last chapter.
	Start, End int
	Progress   func(done, total int, title string, cached bool)
	Notice     func(string)
}

// Run exports a snapshot of the current directory, publishing only a complete TXT.
func Run(ctx context.Context, source Source, bookID string, opt Options) (int, error) {
	if id, err := fanqie.ParseBookID(bookID); err != nil || id != bookID {
		return 0, errors.New("invalid book ID")
	}
	if opt.Output == "" || opt.CacheDir == "" || opt.Interval < 0 {
		return 0, errors.New("output, cache directory and non-negative interval are required")
	}
	if opt.Start < 0 || opt.End < 0 || (opt.End > 0 && opt.End < max(opt.Start, 1)) {
		return 0, errors.New("invalid chapter range")
	}
	if _, err := os.Lstat(opt.Output); err == nil {
		return 0, fmt.Errorf("output already exists: %s (choose another -o path)", opt.Output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	chapters, err := source.Directory(ctx, bookID)
	if err != nil {
		return 0, fmt.Errorf("fetch directory: %w", err)
	}
	if len(chapters) == 0 {
		return 0, errors.New("book has no chapters")
	}
	total, start, end := len(chapters), max(opt.Start, 1), opt.End
	if end == 0 {
		end = total
	}
	if start > end || end > total {
		return 0, fmt.Errorf("chapter range %d-%d exceeds %d chapters", start, end, total)
	}
	chapters = chapters[start-1 : end]
	cache := filepath.Join(opt.CacheDir, bookID)
	if err := os.MkdirAll(cache, 0700); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(opt.Output), 0755); err != nil {
		return 0, err
	}
	file, err := os.CreateTemp(filepath.Dir(opt.Output), ".fanqie-dl-*.tmp")
	if err != nil {
		return 0, err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	w := bufio.NewWriter(file)
	var contents map[string]fanqie.Content
	var cached map[string]bool
	for i, chapter := range chapters {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if i%100 == 0 {
			contents, cached, err = loadBatch(ctx, source, bookID, chapters[i:min(i+100, len(chapters))], cache, opt)
			if err != nil {
				return 0, err
			}
		}
		content := contents[chapter.ID]
		if i == 0 {
			// The batch endpoint omits authors; query once to retain the TXT metadata.
			if content.Author == "" {
				if opt.Notice != nil {
					opt.Notice("批量响应未含作者，查询首章补充书籍信息")
				}
				if err := waitRequest(ctx, opt.Interval); err != nil {
					return 0, err
				}
				meta, err := source.Chapter(ctx, bookID, chapter.ID)
				if err == nil && validContent(meta, bookID, chapter.ID) {
					content.Author = meta.Author
					if err := saveCache(filepath.Join(cache, chapter.ID+".json"), content); err != nil {
						return 0, err
					}
				} else if opt.Notice != nil {
					opt.Notice("作者信息暂不可用，继续导出完整正文")
				}
			}
			count := fmt.Sprint(total)
			if len(chapters) < total {
				count = fmt.Sprintf("第 %d-%d 章（全书 %d 章）", start, end, total)
			}
			if _, err := fmt.Fprintf(w, "%s\n作者：%s\n书籍 ID：%s\n章节数：%s\n\n", content.BookName, content.Author, bookID, count); err != nil {
				return 0, err
			}
		}
		if _, err := fmt.Fprintf(w, "%s\n\n%s\n\n", chapter.Title, content.Text); err != nil {
			return 0, err
		}
		if opt.Progress != nil {
			opt.Progress(i+1, len(chapters), chapter.Title, cached[chapter.ID])
		}
	}
	if err := w.Flush(); err != nil {
		return 0, err
	}
	if err := file.Sync(); err != nil {
		return 0, err
	}
	if err := file.Close(); err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	// A hard link publishes atomically without clobbering a file created during download.
	if err := os.Link(file.Name(), opt.Output); err != nil {
		return 0, fmt.Errorf("publish TXT: %w", err)
	}
	return len(chapters), nil
}

func loadBatch(ctx context.Context, source Source, bookID string, chapters []fanqie.Chapter, cache string, opt Options) (map[string]fanqie.Content, map[string]bool, error) {
	contents := make(map[string]fanqie.Content)
	cached := make(map[string]bool)
	var pending []string
	for _, chapter := range chapters {
		var content fanqie.Content
		raw, err := os.ReadFile(filepath.Join(cache, chapter.ID+".json"))
		if err == nil && json.Unmarshal(raw, &content) == nil && validContent(content, bookID, chapter.ID) {
			contents[chapter.ID], cached[chapter.ID] = content, true
		} else {
			pending = append(pending, chapter.ID)
		}
	}
	for attempt := 0; attempt < 3 && len(pending) > 0; attempt++ {
		if err := waitRequest(ctx, opt.Interval+time.Duration(attempt)*time.Second); err != nil {
			return nil, nil, err
		}
		if opt.Notice != nil {
			opt.Notice(fmt.Sprintf("批量请求：%d 章（第 %d 次）", len(pending), attempt+1))
		}
		result, batchErr := source.Chapters(ctx, bookID, pending)
		var missing []string
		for _, id := range pending {
			content := result[id]
			if !validContent(content, bookID, id) {
				missing = append(missing, id)
				continue
			}
			if err := saveCache(filepath.Join(cache, id+".json"), content); err != nil {
				return nil, nil, fmt.Errorf("save chapter cache: %w", err)
			}
			contents[id] = content
		}
		pending = missing
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if errors.Is(batchErr, context.Canceled) {
			return nil, nil, batchErr
		}
		if len(pending) > 0 && opt.Notice != nil {
			if batchErr == nil {
				batchErr = fmt.Errorf("response is missing %d valid chapters", len(pending))
			}
			opt.Notice(fmt.Sprintf("批量请求未完成：%v", batchErr))
		}
	}
	for _, id := range pending {
		if opt.Notice != nil {
			opt.Notice("单章补取：" + id)
		}
		content, err := fetch(ctx, source, bookID, id, opt.Interval)
		if err != nil {
			return nil, nil, fmt.Errorf("chapter %s: %w", id, err)
		}
		if !validContent(content, bookID, id) {
			return nil, nil, fmt.Errorf("invalid content for chapter %s", id)
		}
		if err := saveCache(filepath.Join(cache, id+".json"), content); err != nil {
			return nil, nil, fmt.Errorf("save chapter cache: %w", err)
		}
		contents[id] = content
	}
	return contents, cached, nil
}

func validContent(c fanqie.Content, bookID, itemID string) bool {
	return c.BookID == bookID && c.ItemID == itemID && strings.TrimSpace(c.BookName) != "" && strings.TrimSpace(c.Text) != ""
}

func fetch(ctx context.Context, source Source, bookID, itemID string, interval time.Duration) (fanqie.Content, error) {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		delay := interval
		if attempt > 0 {
			delay += time.Duration(attempt) * time.Second
		}
		if err := waitRequest(ctx, delay); err != nil {
			return fanqie.Content{}, err
		}
		content, err := source.Chapter(ctx, bookID, itemID)
		if err == nil {
			return content, nil
		}
		if errors.Is(err, context.Canceled) {
			return fanqie.Content{}, err
		}
		last = err
	}
	return fanqie.Content{}, last
}

func waitRequest(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

func saveCache(path string, content fanqie.Content) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".chapter-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := json.NewEncoder(f).Encode(content); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
