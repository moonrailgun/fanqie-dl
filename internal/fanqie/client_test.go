package fanqie

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChapterProtocol(t *testing.T) {
	for _, mode := range []string{"body", "flag", "header", "wrong-book", "missing-key", "empty"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-argus") == "" || r.Header.Get("x-ladon") == "" || r.URL.Query().Get("item_id") != "2" {
					t.Error("missing signature or chapter ID")
				}
				body := "<header>章节名</header><p>完整正文。</p>"
				if mode == "empty" {
					body = "<p></p>"
				}
				peer, encrypted := serverContent(t, r.Header.Get("y"), body)
				if mode != "missing-key" {
					w.Header().Set("y", peer)
				}
				if mode == "flag" {
					w.Header().Set("c", "1")
				}
				if mode == "header" {
					w.Header().Set("c", encrypted)
					encrypted = ""
				}
				bookID := "123"
				if mode == "wrong-book" {
					bookID = "999"
				}
				json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"content": encrypted, "item_id": "2", "title": "章节名", "novel_data": map[string]string{"book_id": bookID, "book_name": "书名", "author": "作者"}}})
			}))
			defer server.Close()
			c := NewClient(time.Second)
			c.contentURL = server.URL
			content, err := c.Chapter(context.Background(), "123", "2")
			if mode == "wrong-book" || mode == "missing-key" || mode == "empty" {
				if err == nil {
					t.Fatal("accepted invalid chapter")
				}
			} else if err != nil || content.Text != "完整正文。" || content.BookName != "书名" {
				t.Fatalf("%+v %v", content, err)
			}
		})
	}
}

func TestSearchAndDirectory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "search") {
			if r.URL.Query().Get("q") != "三国 & 演义" || r.URL.Query().Get("offset") != "10" {
				t.Errorf("bad query: %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"code":0,"data":{"has_more":true,"offset":20,"ret_data":[{"book_id":"123","title":"<em>三国</em>演义","author":"罗贯中"}]}}`)
		} else {
			fmt.Fprint(w, `{"code":0,"data":{"allItemIds":["2","1"],"chapterListWithVolume":[[{"itemId":"1","title":"后章"},{"itemId":"2","title":"前章"}]]}}`)
		}
	}))
	defer server.Close()
	c := NewClient(time.Second)
	c.searchURL = server.URL + "/search"
	c.directoryURL = server.URL + "/directory"
	result, err := c.Search(context.Background(), "三国 & 演义", 10)
	if err != nil || len(result.Books) != 1 || result.Books[0].Title != "三国演义" || result.Offset != 20 || !result.HasMore {
		t.Fatalf("%+v %v", result, err)
	}
	chapters, err := c.Directory(context.Background(), "123")
	if err != nil || len(chapters) != 2 || chapters[0].ID != "2" {
		t.Fatalf("%+v %v", chapters, err)
	}
}

func TestRejectIncompleteDirectory(t *testing.T) {
	for _, data := range []string{
		`{"allItemIds":["1","2"],"chapterListWithVolume":[[{"itemId":"1","title":"one"}]]}`,
		`{"allItemIds":["1","1"],"chapterListWithVolume":[[{"itemId":"1","title":"one"}]]}`,
		`{"allItemIds":[],"chapterListWithVolume":[]}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, `{"code":0,"data":%s}`, data) }))
		c := NewClient(time.Second)
		c.directoryURL = server.URL
		if _, err := c.Directory(context.Background(), "123"); err == nil {
			t.Errorf("accepted incomplete: %s", data)
		}
		server.Close()
	}
}

func TestAPIErrors(t *testing.T) {
	for _, body := range []string{"", `{"code":101,"message":"blocked"}`, `{"data":{}}`, `<html>captcha</html>`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		c := NewClient(time.Second)
		c.searchURL = server.URL
		if _, err := c.Search(context.Background(), "test", 0); err == nil {
			t.Errorf("accepted: %s", body)
		}
		server.Close()
	}
}

func TestCleanContent(t *testing.T) {
	input := `<header><div class="tt-title">标题</div></header><article><style>hidden</style><p>正文 &amp; 内容</p><p>下一行<br/>换行</p></article>`
	if got := cleanContent(input); got != "正文 & 内容\n\n下一行\n\n换行" {
		t.Fatal(got)
	}
	if got := cleanContent(`<header>插图</header><article><img src="https://example.com/a.png?x=1&amp;y=2"></article>`); got != "[图片：https://example.com/a.png?x=1&y=2]" {
		t.Fatal(got)
	}
}

func TestBookID(t *testing.T) {
	for _, input := range []string{"123456", "https://fanqienovel.com/page/123456", "https://fanqienovel.com/reader?book_id=123456"} {
		if id, err := ParseBookID(input); err != nil || id != "123456" {
			t.Fatalf("%s: %s %v", input, id, err)
		}
	}
	for _, input := range []string{"../123", "abc", "https://example.com/page/123456", "https://fanqienovel.com/reader/123456"} {
		if _, err := ParseBookID(input); err == nil {
			t.Fatal(input)
		}
	}
}
