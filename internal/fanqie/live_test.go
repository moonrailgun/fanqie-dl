package fanqie

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLiveAPI(t *testing.T) {
	if os.Getenv("FANQIE_LIVE_TEST") != "1" {
		t.Skip("set FANQIE_LIVE_TEST=1 to contact upstream")
	}
	c := NewClient(20 * time.Second)
	ctx := context.Background()
	result, err := c.Search(ctx, "三国演义", 0)
	if err != nil || len(result.Books) == 0 {
		t.Fatalf("search: %+v %v", result, err)
	}
	chapters, err := c.Directory(ctx, "6838936284967209991")
	if err != nil || len(chapters) < 120 {
		t.Fatalf("directory: %d %v", len(chapters), err)
	}
	ch, err := c.Chapter(ctx, "6838936284967209991", "6838937003908661773")
	if err != nil {
		t.Fatal(err)
	}
	if ch.BookName != "三国演义" || !strings.Contains(ch.Text, "曹操") {
		t.Fatalf("unexpected chapter: %+v", ch)
	}
	t.Logf("search=%d books, directory=%d chapters, decoded chapter=%d bytes", len(result.Books), len(chapters), len(ch.Text))
}

func TestLiveBatchAPI(t *testing.T) {
	if os.Getenv("FANQIE_LIVE_TEST") != "1" {
		t.Skip("set FANQIE_LIVE_TEST=1 to contact upstream")
	}
	c := NewClient(20 * time.Second)
	ids := []string{"6838937002725868045", "6921689570094353422", "6838937003908661773"}
	chapters, err := c.Chapters(context.Background(), "6838936284967209991", ids)
	if err != nil || len(chapters) != len(ids) {
		t.Fatalf("batch=%d err=%v", len(chapters), err)
	}
	if !strings.Contains(chapters[ids[2]].Text, "曹操") {
		t.Fatal("unexpected decrypted batch content")
	}
	t.Logf("one full/mget request decrypted %d chapters", len(chapters))
}
