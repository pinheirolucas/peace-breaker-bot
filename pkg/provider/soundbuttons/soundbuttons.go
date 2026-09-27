// Package soundbuttons scrapes soundbuttons.io's listing pages.
//
// Each card's name and URL ship as a double-escaped JSON.parse('...') argument in an Alpine.js @click
// attribute. Search (/search?q=&page=N) is plain paginated HTML. Browsing serves only page 1 at
// /trending; deeper pages come from the undocumented /api/feed/trending?sort=trending&page=N feed,
// which degrades to an empty page if its shape changes.
package soundbuttons

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/httpclient"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/provider"
)

const (
	key         = "soundbuttons"
	displayName = "Sound Buttons"

	pageSize = 60
)

var (
	defaultClient = httpclient.New()

	clickPayloadPattern = regexp.MustCompile(`\$store\.player\.play\(JSON\.parse\('(.*?)'\)\)`)
)

// Provider scrapes soundbuttons.io. BaseURL and Client default to
// production values when unset, so a test can point both at an httptest
// server.
type Provider struct {
	BaseURL string
	Client  *http.Client
}

func New() *Provider { return &Provider{} }

func (p *Provider) Key() string         { return key }
func (p *Provider) DisplayName() string { return displayName }

func (p *Provider) AllowedContentHosts() []string {
	return []string{"cdn.soundbuttons.io"}
}

func (p *Provider) SupportsRegion() bool { return false }

func (p *Provider) baseURL() string {
	if p.BaseURL != "" {
		return p.BaseURL
	}

	return "https://soundbuttons.io"
}

func (p *Provider) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}

	return defaultClient
}

// List searches via plain, server-paginated HTML. Browsing uses /trending
// for page 1, but /trending 404s on any ?page= — deeper pages go through
// an undocumented JSON feed endpoint instead, and degrade to an empty page
// if that endpoint ever changes shape.
func (p *Provider) List(params provider.ListParams) (*provider.ListResult, error) {
	page := params.Page
	if page < 1 {
		page = 1
	}

	search := strings.TrimSpace(params.Search)

	if search != "" {
		listURL := p.baseURL() + "/search?q=" + url.QueryEscape(search) + "&page=" + strconv.Itoa(page)
		return p.fetchHTML(listURL, page)
	}

	if page == 1 {
		return p.fetchHTML(p.baseURL()+"/trending", page)
	}

	return p.fetchFeed(page)
}

func (p *Provider) fetchHTML(listURL string, page int) (*provider.ListResult, error) {
	res, err := p.client().Get(listURL)
	if err != nil {
		return nil, fmt.Errorf("soundbuttons: %w: %v", provider.ErrUpstreamUnavailable, err)
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		slog.Debug("upstream 404, returning an empty page", "provider", key, "url", listURL)
		return emptyPage(page), nil
	default:
		return nil, fmt.Errorf("soundbuttons: %w: status %d", provider.ErrBadUpstreamStatus, res.StatusCode)
	}

	return parseList(res.Body, page)
}

type feedResponse struct {
	HTML string `json:"html"`
}

func (p *Provider) fetchFeed(page int) (*provider.ListResult, error) {
	feedURL := p.baseURL() + "/api/feed/trending?sort=trending&page=" + strconv.Itoa(page)

	res, err := p.client().Get(feedURL)
	if err != nil {
		return nil, fmt.Errorf("soundbuttons: %w: %v", provider.ErrUpstreamUnavailable, err)
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		slog.Debug("upstream 404, returning an empty page", "provider", key, "url", feedURL)
		return emptyPage(page), nil
	default:
		return nil, fmt.Errorf("soundbuttons: %w: status %d", provider.ErrBadUpstreamStatus, res.StatusCode)
	}

	var feed feedResponse
	if err := json.NewDecoder(res.Body).Decode(&feed); err != nil {
		slog.Warn("soundbuttons trending feed undecodable, returning an empty page", "page", page, "err", err)
		return emptyPage(page), nil
	}

	return parseList(strings.NewReader(feed.HTML), page)
}

func emptyPage(page int) *provider.ListResult {
	return &provider.ListResult{
		Instants: []provider.Instant{},
		Pages:    provider.InferPages(page, 0, pageSize),
	}
}

type card struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

func parseList(r io.Reader, page int) (*provider.ListResult, error) {
	document, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}

	instants := []provider.Instant{}
	malformed := 0

	document.Find("article.sc").Each(func(i int, article *goquery.Selection) {
		click, ok := article.Attr("@click")
		if !ok {
			malformed++
			return
		}

		match := clickPayloadPattern.FindStringSubmatch(click)
		if match == nil {
			malformed++
			return
		}

		var c card
		if err := json.Unmarshal([]byte(unescapeJSString(match[1])), &c); err != nil || c.Title == "" || c.URL == "" {
			malformed++
			return
		}

		instants = append(instants, provider.Instant{Name: c.Title, URL: c.URL})
	})

	if malformed > 0 {
		return nil, fmt.Errorf("soundbuttons: %w: %d card(s) with an unreadable @click payload", provider.ErrUnexpectedMarkup, malformed)
	}

	return &provider.ListResult{
		Instants: instants,
		Pages:    provider.InferPages(page, len(instants), pageSize),
	}, nil
}

func unescapeJSString(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}

		next := s[i+1]
		if next == 'u' && i+5 < len(s) {
			if n, err := strconv.ParseUint(s[i+2:i+6], 16, 32); err == nil {
				b.WriteRune(rune(n))
				i += 5
				continue
			}
		}

		b.WriteByte(next)
		i++
	}

	return b.String()
}
