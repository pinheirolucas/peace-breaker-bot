// Package instantsmeme scrapes instants.meme's listing pages.
package instantsmeme

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/httpclient"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/provider"
)

const (
	key         = "instantsmeme"
	displayName = "InstantsMeme"

	// Confirmed against both /popular/ and a search: a full page is 40
	// results.
	pageSize = 40
)

var defaultClient = httpclient.New()

// Provider scrapes instants.meme. BaseURL and Client default to production
// values when unset, so a test can point both at an httptest server.
type Provider struct {
	BaseURL string
	Client  *http.Client
}

func New() *Provider { return &Provider{} }

func (p *Provider) Key() string         { return key }
func (p *Provider) DisplayName() string { return displayName }

func (p *Provider) AllowedContentHosts() []string {
	return []string{"cdn.instants.meme"}
}

func (p *Provider) baseURL() string {
	if p.BaseURL != "" {
		return p.BaseURL
	}

	return "https://instants.meme"
}

func (p *Provider) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}

	return defaultClient
}

// List browses /popular/ (no search) or searches by name. Category pages
// exist but don't actually honor a page query param — /popular/ does, so
// it's used as the browse endpoint instead.
func (p *Provider) List(params provider.ListParams) (*provider.ListResult, error) {
	page := params.Page
	if page < 1 {
		page = 1
	}

	search := strings.TrimSpace(params.Search)

	var listURL string
	if search != "" {
		listURL = p.baseURL() + "/?s=" + url.QueryEscape(search) + "&paged=" + strconv.Itoa(page)
	} else {
		listURL = p.baseURL() + "/popular/?page=" + strconv.Itoa(page)
	}

	res, err := p.client().Get(listURL)
	if err != nil {
		return nil, fmt.Errorf("instantsmeme: %w: %v", provider.ErrUpstreamUnavailable, err)
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		// continue
	case http.StatusNotFound:
		return emptyPage(page), nil
	default:
		return nil, fmt.Errorf("instantsmeme: %w: status %d", provider.ErrBadUpstreamStatus, res.StatusCode)
	}

	// /popular/ silently redirects an out-of-range page back to page 1
	// instead of answering 404 — treat a page that didn't survive the
	// redirect as past the end, the same as a 404, rather than serve page
	// 1's clips back under the wrong page number.
	if page > 1 && search == "" && res.Request != nil && res.Request.URL.Query().Get("page") != strconv.Itoa(page) {
		return emptyPage(page), nil
	}

	return parseList(res.Body, page)
}

func emptyPage(page int) *provider.ListResult {
	return &provider.ListResult{
		Instants: []provider.Instant{},
		Pages:    provider.InferPages(page, 0, pageSize),
	}
}

func parseList(r io.Reader, page int) (*provider.ListResult, error) {
	document, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}

	instants := []provider.Instant{}
	malformed := 0

	document.Find("button.sound-btn[data-src]").Each(func(i int, btn *goquery.Selection) {
		src, hasSrc := btn.Attr("data-src")
		name, hasTitle := btn.Attr("title")
		if !hasSrc || !hasTitle || src == "" || name == "" {
			malformed++
			return
		}

		instants = append(instants, provider.Instant{Name: name, URL: src})
	})

	if malformed > 0 {
		return nil, fmt.Errorf("instantsmeme: %w: %d button(s) missing data-src/title", provider.ErrUnexpectedMarkup, malformed)
	}

	return &provider.ListResult{
		Instants: instants,
		Pages:    provider.InferPages(page, len(instants), pageSize),
	}, nil
}
