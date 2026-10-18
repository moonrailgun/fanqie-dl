package fanqie

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBatchProtocol(t *testing.T) {
	for _, mode := range []string{"map", "array", "missing", "wrong-book", "bad-cipher", "unavailable", "empty", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				digest := md5.Sum(raw)
				if r.Method != http.MethodPost || r.URL.Query().Get("aid") != "3040" || r.Header.Get("x-ss-stub") != hex.EncodeToString(digest[:]) || r.Header.Get("x-argus") == "" {
					t.Error("invalid signed batch request")
				}
				var body struct {
					BookID string   `json:"book_id"`
					IDs    []string `json:"item_ids"`
					Key    string   `json:"key"`
				}
				if err := json.Unmarshal(raw, &body); err != nil {
					t.Fatal(err)
				}
				if body.BookID != "123" || len(body.IDs) != 2 || body.IDs[0] != "2" || body.IDs[1] != "1" {
					t.Errorf("unexpected batch: %+v", body)
				}
				items := make(map[string]batchItem)
				zero := 0
				for _, id := range body.IDs {
					peer, ct := serverContent(t, body.Key, "<h1>章节"+id+"</h1><p>正文"+id+"</p>")
					items[id] = batchItem{Code: &zero, Key: peer, Content: ct, Title: "章节" + id, Novel: Content{BookID: "123", BookName: "书名", Author: "作者", ItemID: id}}
				}
				item := items["1"]
				switch mode {
				case "wrong-book":
					item.Novel.BookID = "999"
				case "bad-cipher":
					item.Content = "AAAA"
				case "unavailable":
					code := 101
					item.Code = &code
				case "empty":
					item.Key, item.Content = serverContent(t, body.Key, "<h1>章节1</h1>")
				}
				items["1"] = item
				if mode == "missing" {
					delete(items, "1")
				}
				var value any = items
				if mode == "array" {
					value = []batchItem{items["1"], items["2"]}
				}
				if mode == "duplicate" {
					value = []batchItem{items["2"], items["2"]}
				}
				json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"item_infos": value}})
			}))
			defer server.Close()
			c := NewClient(time.Second)
			c.fullURL = server.URL
			got, err := c.Chapters(context.Background(), "123", []string{"2", "1"})
			if mode == "map" || mode == "array" {
				if err != nil || len(got) != 2 || got["1"].Text != "正文1" {
					t.Fatalf("%+v %v", got, err)
				}
			} else if mode == "duplicate" {
				if err == nil {
					t.Fatal("accepted duplicate response")
				}
			} else if err == nil || len(got) != 1 || got["2"].Text != "正文2" {
				t.Fatalf("must preserve only valid partial result: %+v %v", got, err)
			}
		})
	}
}

func TestBatchInvalidIDs(t *testing.T) {
	c := NewClient(time.Second)
	for _, ids := range [][]string{nil, {"1", "1"}, {"../1"}, make([]string, 3001)} {
		if _, err := c.Chapters(context.Background(), "123", ids); err == nil {
			t.Fatal("accepted invalid IDs")
		}
	}
}
