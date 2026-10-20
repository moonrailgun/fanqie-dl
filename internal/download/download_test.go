package download

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fanqie-dl/internal/fanqie"
)

type fakeSource struct {
	calls         []string
	fail          string
	failOnce      bool
	batches       [][]string
	omit          string
	chapters      []fanqie.Chapter
	noBatchAuthor bool
	singleErr     error
}

func (s *fakeSource) Directory(context.Context, string) ([]fanqie.Chapter, error) {
	if s.chapters != nil {
		return s.chapters, nil
	}
	return []fanqie.Chapter{{ID: "2", Title: "第一章"}, {ID: "1", Title: "第二章"}}, nil
}

func (s *fakeSource) Chapters(ctx context.Context, bookID string, ids []string) (map[string]fanqie.Content, error) {
	s.batches = append(s.batches, append([]string(nil), ids...))
	if s.failOnce {
		s.failOnce = false
		return nil, errors.New("temporary batch failure")
	}
	result := make(map[string]fanqie.Content)
	for _, id := range ids {
		if id == s.fail {
			return result, context.Canceled
		}
		if id == s.omit {
			continue
		}
		result[id] = fanqie.Content{BookID: bookID, BookName: "测试书", Author: "作者", ItemID: id, Text: "正文" + id}
		if s.noBatchAuthor {
			content := result[id]
			content.Author = ""
			result[id] = content
		}
	}
	return result, nil
}

func (s *fakeSource) Chapter(ctx context.Context, bookID, itemID string) (fanqie.Content, error) {
	s.calls = append(s.calls, itemID)
	if s.singleErr != nil {
		return fanqie.Content{}, s.singleErr
	}
	if s.failOnce {
		s.failOnce = false
		return fanqie.Content{}, errors.New("temporary upstream failure")
	}
	if s.fail == itemID {
		return fanqie.Content{}, context.Canceled
	}
	return fanqie.Content{BookID: bookID, BookName: "测试书", Author: "作者", ItemID: itemID, Text: "正文" + itemID}, nil
}

func TestBatchMetadataAndCachedReuse(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			dir := t.TempDir()
			source := &fakeSource{noBatchAuthor: true}
			if fail {
				source.singleErr = errors.New("metadata unavailable")
			}
			opt := Options{Output: filepath.Join(dir, "book.txt"), CacheDir: filepath.Join(dir, "cache")}
			if _, err := Run(context.Background(), source, "123", opt); err != nil {
				t.Fatal(err)
			}
			if strings.Join(source.calls, ",") != "2" {
				t.Fatalf("metadata calls: %v", source.calls)
			}
			data, _ := os.ReadFile(opt.Output)
			if !strings.Contains(string(data), "正文2") || (!fail && !strings.Contains(string(data), "作者：作者")) {
				t.Fatalf("incomplete output: %s", data)
			}
			if !fail {
				source.calls, source.batches = nil, nil
				opt.Output = filepath.Join(dir, "cached.txt")
				if _, err := Run(context.Background(), source, "123", opt); err != nil {
					t.Fatal(err)
				}
				if len(source.calls) != 0 || len(source.batches) != 0 {
					t.Fatal("requested cached content")
				}
			}
		})
	}
}

func TestFallbackFailureKeepsCacheWithoutPublishing(t *testing.T) {
	dir := t.TempDir()
	source := &fakeSource{omit: "1", singleErr: context.Canceled}
	opt := Options{Output: filepath.Join(dir, "book.txt"), CacheDir: filepath.Join(dir, "cache")}
	if _, err := Run(context.Background(), source, "123", opt); err == nil {
		t.Fatal("accepted missing chapter")
	}
	if _, err := os.Stat(opt.Output); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("published partial output")
	}
	if _, err := os.Stat(filepath.Join(opt.CacheDir, "123", "2.json")); err != nil {
		t.Fatal("lost successful batch result")
	}
	if strings.Join(source.calls, ",") != "1" {
		t.Fatal("did not attempt single fallback")
	}
}

func TestRetryAndConcurrentOutputProtection(t *testing.T) {
	dir := t.TempDir()
	opt := Options{Output: filepath.Join(dir, "book.txt"), CacheDir: filepath.Join(dir, "cache")}
	opt.Progress = func(done, total int, title string, cached bool) {
		if done == total {
			if err := os.WriteFile(opt.Output, []byte("another process"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	source := &fakeSource{failOnce: true}
	if _, err := Run(context.Background(), source, "123", opt); err == nil {
		t.Fatal("overwrote concurrent output")
	}
	if len(source.batches) != 2 || len(source.calls) != 0 {
		t.Fatalf("batch retry: %v, single calls: %v", source.batches, source.calls)
	}
	data, err := os.ReadFile(opt.Output)
	if err != nil || string(data) != "another process" {
		t.Fatalf("%q %v", data, err)
	}
}

func TestResumeAndOrderedOutput(t *testing.T) {
	dir := t.TempDir()
	opt := Options{Output: filepath.Join(dir, "book.txt"), CacheDir: filepath.Join(dir, "cache")}
	source := &fakeSource{fail: "1"}
	if _, err := Run(context.Background(), source, "123", opt); err == nil {
		t.Fatal("expected interrupted download")
	}
	if _, err := os.Stat(opt.Output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial final file exists: %v", err)
	}
	source.fail, source.calls = "", nil
	source.batches = nil
	if n, err := Run(context.Background(), source, "123", opt); err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}
	if len(source.batches) != 1 || strings.Join(source.batches[0], ",") != "1" || len(source.calls) != 0 {
		t.Fatalf("did not resume: %v", source.batches)
	}
	b, err := os.ReadFile(opt.Output)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "测试书\n作者：作者\n书籍 ID：123\n章节数：2\n\n第一章\n\n正文2\n\n第二章\n\n正文1\n\n" {
		t.Fatalf("unexpected TXT: %s", b)
	}
	if _, err := Run(context.Background(), source, "123", opt); err == nil {
		t.Fatal("overwrote an existing book")
	}
	after, _ := os.ReadFile(opt.Output)
	if string(after) != string(b) {
		t.Fatal("existing book changed")
	}
}

func TestCorruptCacheRefetched(t *testing.T) {
	dir := t.TempDir()
	opt := Options{Output: filepath.Join(dir, "book.txt"), CacheDir: filepath.Join(dir, "cache")}
	bookCache := filepath.Join(opt.CacheDir, "123")
	if err := os.MkdirAll(bookCache, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bookCache, "2.json"), []byte(`{"book_id":"999","item_id":"2","text":"wrong book"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bookCache, "1.json"), []byte(`broken`), 0600); err != nil {
		t.Fatal(err)
	}
	source := &fakeSource{}
	if _, err := Run(context.Background(), source, "123", opt); err != nil {
		t.Fatal(err)
	}
	if len(source.batches) != 1 || strings.Join(source.batches[0], ",") != "2,1" || len(source.calls) != 0 {
		t.Fatalf("reused invalid cache: %v", source.batches)
	}
}

func TestPartialBatchRetriesOnlyMissingThenFallsBack(t *testing.T) {
	dir := t.TempDir()
	var notices []string
	opt := Options{Output: filepath.Join(dir, "book.txt"), CacheDir: filepath.Join(dir, "cache"), Notice: func(s string) { notices = append(notices, s) }}
	source := &fakeSource{omit: "1"}
	if n, err := Run(context.Background(), source, "123", opt); err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}
	if fmt.Sprint(source.batches) != "[[2 1] [1] [1]]" || strings.Join(source.calls, ",") != "1" {
		t.Fatalf("batch=%v single=%v", source.batches, source.calls)
	}
	if !strings.Contains(strings.Join(notices, "\n"), "单章补取") {
		t.Fatal("fallback was silent")
	}
}

func TestBatchBoundaries(t *testing.T) {
	dir := t.TempDir()
	source := &fakeSource{}
	for i := 1; i <= 201; i++ {
		source.chapters = append(source.chapters, fanqie.Chapter{ID: fmt.Sprint(i), Title: fmt.Sprint("章", i)})
	}
	opt := Options{Output: filepath.Join(dir, "book.txt"), CacheDir: filepath.Join(dir, "cache")}
	if n, err := Run(context.Background(), source, "123", opt); err != nil || n != 201 {
		t.Fatalf("%d %v", n, err)
	}
	if len(source.batches) != 3 || len(source.batches[0]) != 100 || len(source.batches[1]) != 100 || len(source.batches[2]) != 1 || len(source.calls) != 0 {
		t.Fatalf("batch sizes/calls wrong: %v", source.batches)
	}
}

func TestCanceledContext(t *testing.T) {
	dir := t.TempDir()
	opt := Options{Output: filepath.Join(dir, "book.txt"), CacheDir: filepath.Join(dir, "cache")}
	source := &fakeSource{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Run(ctx, source, "123", opt)
	if err == nil || len(source.calls) != 0 {
		t.Fatalf("%v calls=%v", err, source.calls)
	}
	if _, err := os.Stat(opt.Output); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("published incomplete book")
	}
}

func TestChapterRange(t *testing.T) {
	dir := t.TempDir()
	source := &fakeSource{chapters: []fanqie.Chapter{{ID: "1", Title: "一"}, {ID: "2", Title: "二"}, {ID: "3", Title: "三"}}}
	var totals []int
	opt := Options{Output: filepath.Join(dir, "book.txt"), CacheDir: filepath.Join(dir, "cache"), Start: 2,
		Progress: func(done, total int, title string, cached bool) { totals = append(totals, total) }}
	if n, err := Run(context.Background(), source, "123", opt); err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}
	if len(source.batches) != 1 || strings.Join(source.batches[0], ",") != "2,3" || fmt.Sprint(totals) != "[2 2]" {
		t.Fatalf("batches %v, totals %v", source.batches, totals)
	}
	b, _ := os.ReadFile(opt.Output)
	if string(b) != "测试书\n作者：作者\n书籍 ID：123\n章节数：第 2-3 章（全书 3 章）\n\n二\n\n正文2\n\n三\n\n正文3\n\n" {
		t.Fatalf("unexpected TXT: %s", b)
	}
	for _, r := range [][2]int{{1, 4}, {4, 0}, {3, 2}, {-1, 0}} {
		opt.Output, opt.Start, opt.End = filepath.Join(dir, fmt.Sprint(r)+".txt"), r[0], r[1]
		if _, err := Run(context.Background(), source, "123", opt); err == nil {
			t.Fatalf("accepted range %v", r)
		}
	}
}
