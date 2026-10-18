// SPDX-License-Identifier: GPL-3.0-only
// Adapted from ch6vip/fqapp rust/src/endpoints/full.rs and upstream/cm.rs.
package fanqie

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type batchItem struct {
	Code    *int    `json:"code"`
	Key     string  `json:"key"`
	Content string  `json:"content"`
	ItemID  string  `json:"item_id"`
	Title   string  `json:"title"`
	Novel   Content `json:"novel_data"`
}

// Chapters returns successfully decrypted chapters even when part of a batch fails.
func (c *Client) Chapters(ctx context.Context, bookID string, itemIDs []string) (map[string]Content, error) {
	if !numericID.MatchString(bookID) || len(itemIDs) == 0 || len(itemIDs) > 3000 {
		return nil, errors.New("batch requires a book ID and 1–3000 chapter IDs")
	}
	requested := make(map[string]bool, len(itemIDs))
	for _, id := range itemIDs {
		if !numericID.MatchString(id) || requested[id] {
			return nil, errors.New("invalid or duplicate batch chapter ID")
		}
		requested[id] = true
	}
	// CM uses unsigned public-key bytes; the single-chapter API uses Java encoding.
	key, state, err := newDHHandshake(false)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{"book_id": bookID, "item_ids": itemIDs, "key": key})
	if err != nil {
		return nil, err
	}
	q := url.Values{"aid": {"3040"}, "app_name": {"novelapp"}, "channel": {"0"}, "device_id": {"0"}, "device_platform": {"android"}, "device_type": {"UGFFI55"}, "iid": {"0"}, "os_version": {"0"}, "version_code": {"58932"}, "version_name": {"5.8.9.32"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.fullURL+"?"+q.Encode(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "com.dragon.read")
	if err := sign(req, body); err != nil {
		return nil, err
	}
	var response struct {
		Data struct {
			Items json.RawMessage `json:"item_infos"`
		} `json:"data"`
	}
	if _, err := c.do(req, &response); err != nil {
		return nil, err
	}
	raw := bytes.TrimSpace(response.Data.Items)
	items := make(map[string]batchItem)
	if len(raw) > 0 && raw[0] == '[' {
		var list []batchItem
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, fmt.Errorf("decode batch items: %w", err)
		}
		for _, item := range list {
			id := item.ItemID
			if id == "" {
				id = item.Novel.ItemID
			}
			if _, exists := items[id]; exists {
				return nil, errors.New("duplicate chapter in batch response")
			}
			items[id] = item
		}
	} else if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("decode batch items: %w", err)
	}
	result := make(map[string]Content)
	var failures []error
	for _, id := range itemIDs {
		item, ok := items[id]
		if !ok || item.Code == nil || *item.Code != 0 || item.Novel.BookID != bookID || item.Novel.ItemID != id || (item.ItemID != "" && item.ItemID != id) {
			failures = append(failures, fmt.Errorf("chapter %s missing, unavailable or mismatched", id))
			continue
		}
		plain, err := state.decrypt(item.Key, item.Content)
		if err != nil {
			failures = append(failures, fmt.Errorf("decrypt chapter %s: %w", id, err))
			continue
		}
		content := item.Novel
		content.BookName, content.Author = plainText(content.BookName), plainText(content.Author)
		content.Title, content.Text = plainText(item.Title), cleanContent(plain)
		if first, rest, _ := strings.Cut(content.Text, "\n\n"); first == content.Title {
			content.Text = rest
		}
		if strings.TrimSpace(content.BookName) == "" || content.Text == "" {
			failures = append(failures, fmt.Errorf("chapter %s has no book name or text", id))
			continue
		}
		result[id] = content
	}
	if len(failures) > 0 {
		return result, fmt.Errorf("batch incomplete (%d/%d chapters): %w", len(result), len(itemIDs), failures[0])
	}
	return result, nil
}
