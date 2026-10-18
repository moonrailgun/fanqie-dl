// SPDX-License-Identifier: GPL-3.0-only
// Endpoint contracts adapted from ch6vip/fqapp rust/src/endpoints.
package fanqie

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Book struct {
	ID       string `json:"book_id"`
	Title    string `json:"title"`
	Author   string `json:"author"`
	Category string `json:"category,omitempty"`
	Abstract string `json:"abstract,omitempty"`
}

type SearchResult struct {
	Books   []Book `json:"books"`
	HasMore bool   `json:"has_more"`
	Offset  int    `json:"offset"`
}

type Chapter struct {
	ID    string `json:"itemId"`
	Title string `json:"title"`
}

type Content struct {
	BookID   string `json:"book_id"`
	BookName string `json:"book_name"`
	Author   string `json:"author"`
	ItemID   string `json:"item_id"`
	Title    string `json:"title"`
	Text     string `json:"text"`
}

type Client struct {
	http                                         *http.Client
	searchURL, directoryURL, contentURL, fullURL string
}

func NewClient(timeout time.Duration) *Client {
	return &Client{http: &http.Client{Timeout: timeout}, searchURL: "https://api.fanqiesdk.com/api/novel/channel/homepage/search/search/v1/", directoryURL: "https://fanqienovel.com/api/reader/directory/detail", contentURL: "https://novel.snssdk.com/api/novel/book/reader/content/v1", fullURL: "https://novelfm-hl.snssdk.com/novelfm/playerapi/full/mget/v1/"}
}

func (c *Client) get(ctx context.Context, base string, q url.Values, y string, dst any) (http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "com.ss.android.article.news/13400")
	if y != "" {
		req.Header.Set("y", y)
	}
	if err := sign(req, nil); err != nil {
		return nil, err
	}
	return c.do(req, dst)
}

func (c *Client) do(req *http.Request, dst any) (http.Header, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream HTTP %d", resp.StatusCode)
	}
	const maxResponse = 32 << 20
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, errors.New("upstream returned an empty response (request may be blocked)")
	}
	if len(raw) > maxResponse {
		return nil, errors.New("upstream response exceeds 32 MiB")
	}
	var envelope struct {
		Code    *int   `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("invalid upstream JSON: %w", err)
	}
	if envelope.Code == nil {
		return nil, errors.New("upstream JSON is missing code")
	}
	if *envelope.Code != 0 {
		return nil, fmt.Errorf("upstream code %d: %.200s", *envelope.Code, envelope.Message)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return nil, fmt.Errorf("decode upstream data: %w", err)
	}
	return resp.Header, nil
}

func (c *Client) Search(ctx context.Context, query string, offset int) (SearchResult, error) {
	if strings.TrimSpace(query) == "" || offset < 0 {
		return SearchResult{}, errors.New("search requires a keyword and non-negative offset")
	}
	var response struct {
		Data *struct {
			Books   []Book `json:"ret_data"`
			HasMore bool   `json:"has_more"`
			Offset  int    `json:"offset"`
		} `json:"data"`
	}
	_, err := c.get(ctx, c.searchURL, url.Values{"q": {query}, "offset": {strconv.Itoa(offset)}, "aid": {"13"}, "app_name": {"news_article"}, "version_code": {"708"}, "device_platform": {"android"}}, "", &response)
	if err != nil {
		return SearchResult{}, err
	}
	if response.Data == nil || response.Data.Books == nil {
		return SearchResult{}, errors.New("search response is missing results")
	}
	for i := range response.Data.Books {
		b := &response.Data.Books[i]
		b.Title = plainText(b.Title)
		b.Author = plainText(b.Author)
		if !numericID.MatchString(b.ID) {
			return SearchResult{}, errors.New("invalid book ID in search response")
		}
	}
	return SearchResult{response.Data.Books, response.Data.HasMore, response.Data.Offset}, nil
}

func (c *Client) Directory(ctx context.Context, bookID string) ([]Chapter, error) {
	if !numericID.MatchString(bookID) {
		return nil, errors.New("invalid book ID")
	}
	var response struct {
		Data struct {
			IDs     []string    `json:"allItemIds"`
			Volumes [][]Chapter `json:"chapterListWithVolume"`
		} `json:"data"`
	}
	_, err := c.get(ctx, c.directoryURL, url.Values{"bookId": {bookID}}, "", &response)
	if err != nil {
		return nil, err
	}
	if len(response.Data.IDs) == 0 {
		return nil, errors.New("book has no available directory")
	}
	byID := make(map[string]Chapter)
	for _, volume := range response.Data.Volumes {
		for _, chapter := range volume {
			if _, exists := byID[chapter.ID]; exists {
				return nil, errors.New("duplicate chapter in directory")
			}
			byID[chapter.ID] = chapter
		}
	}
	ordered := make([]Chapter, 0, len(response.Data.IDs))
	for _, id := range response.Data.IDs {
		ch, ok := byID[id]
		if !ok || !numericID.MatchString(id) || strings.TrimSpace(ch.Title) == "" {
			return nil, fmt.Errorf("incomplete directory at chapter %q", id)
		}
		ordered = append(ordered, ch)
		delete(byID, id)
	}
	if len(byID) != 0 {
		return nil, errors.New("directory chapter lists disagree")
	}
	return ordered, nil
}

func (c *Client) Chapter(ctx context.Context, bookID, itemID string) (Content, error) {
	if !numericID.MatchString(bookID) || !numericID.MatchString(itemID) {
		return Content{}, errors.New("invalid book or chapter ID")
	}
	y, state, err := newHandshake()
	if err != nil {
		return Content{}, err
	}
	q := url.Values{"aid": {"13"}, "app_name": {"novelapp"}, "channel": {"0"}, "device_platform": {"android"}, "device_type": {"25053RT47C"}, "item_id": {itemID}, "os_version": {"10"}, "support_image": {"1"}, "version_code": {"66.9"}}
	var response struct {
		Data struct {
			Content string  `json:"content"`
			ItemID  string  `json:"item_id"`
			Title   string  `json:"title"`
			Novel   Content `json:"novel_data"`
		} `json:"data"`
	}
	headers, err := c.get(ctx, c.contentURL, q, y, &response)
	if err != nil {
		return Content{}, err
	}
	data := response.Data
	if data.ItemID != itemID || data.Novel.BookID != bookID {
		return Content{}, errors.New("chapter response belongs to another book or chapter")
	}
	encrypted := headers.Get("c")
	if encrypted == "" || encrypted == "1" {
		encrypted = data.Content
	}
	if headers.Get("y") == "" {
		return Content{}, errors.New("chapter response is missing its decryption key")
	}
	body, err := state.decrypt(headers.Get("y"), encrypted)
	if err != nil {
		return Content{}, fmt.Errorf("decrypt chapter: %w", err)
	}
	text := cleanContent(body)
	if text == "" {
		return Content{}, errors.New("chapter contains no text")
	}
	result := data.Novel
	result.BookName = plainText(result.BookName)
	result.Author = plainText(result.Author)
	result.ItemID = itemID
	result.Title = plainText(data.Title)
	result.Text = text
	return result, nil
}

var numericID = regexp.MustCompile(`^[0-9]{1,30}$`)
var pagePath = regexp.MustCompile(`^/page/([0-9]+)/?$`)

func ParseBookID(input string) (string, error) {
	input = strings.TrimSpace(input)
	if numericID.MatchString(input) {
		return input, nil
	}
	u, err := url.Parse(input)
	if err != nil || u.Scheme != "https" || (u.Hostname() != "fanqienovel.com" && u.Hostname() != "www.fanqienovel.com") {
		return "", errors.New("expected a numeric book ID or https://fanqienovel.com/page/BOOK_ID")
	}
	if match := pagePath.FindStringSubmatch(u.Path); match != nil && numericID.MatchString(match[1]) {
		return match[1], nil
	}
	for _, key := range []string{"book_id", "bookId"} {
		if id := u.Query().Get(key); numericID.MatchString(id) {
			return id, nil
		}
	}
	return "", errors.New("link has no book ID; a reader chapter ID is not a book ID")
}

var tags = regexp.MustCompile(`(?s)<[^>]*>`)
var hidden = regexp.MustCompile(`(?is)<(?:header|head|script|style)\b[^>]*>.*?</(?:header|head|script|style)\s*>|<!--.*?-->`)
var breaks = regexp.MustCompile(`(?i)</?(?:p|div|br|h[1-6]|section|article)\b[^>]*>`)
var images = regexp.MustCompile(`(?is)<img\b[^>]*>`)
var imageSource = regexp.MustCompile(`(?i)\bsrc\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)

func plainText(s string) string {
	return strings.TrimSpace(html.UnescapeString(tags.ReplaceAllString(s, "")))
}
func cleanContent(s string) string {
	s = hidden.ReplaceAllString(s, "")
	s = images.ReplaceAllStringFunc(s, func(tag string) string {
		if match := imageSource.FindStringSubmatch(tag); match != nil {
			return "\n[图片：" + match[1] + match[2] + match[3] + "]\n"
		}
		return "\n[图片]\n"
	})
	s = breaks.ReplaceAllString(s, "\n")
	s = html.UnescapeString(tags.ReplaceAllString(s, ""))
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n\n")
}
