// Package soundboardguy scrapes soundboardguy.com's listing pages.
//
// Each a.shareable--trigger carries the sound name and, in data-audio, the id of the audio element
// whose source is the clip URL. Browsing uses /sounds/page/N/ and search uses WordPress's /?s=TERM,
// with different page sizes. Every page, even a zero-result search, also renders an unrelated
// recommendations grid with the same markup under an extra --infinite class, so parsing must scope
// to the grid without it.
package soundboardguy

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/httpclient"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/provider"
)

const (
	key         = "soundboardguy"
	displayName = "SoundboardGuy"

	browsePageSize = 40
	searchPageSize = 10
)

var defaultClient = httpclient.New()

// Provider scrapes soundboardguy.com. BaseURL and Client default to
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
	return []string{"soundboardguy.com"}
}

func (p *Provider) SupportsRegion() bool { return false }

func (p *Provider) baseURL() string {
	if p.BaseURL != "" {
		return p.BaseURL
	}

	return "https://soundboardguy.com"
}

func (p *Provider) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}

	return defaultClient
}

// List browses /sounds/ (no search) or searches by name; category pages
// also exist and paginate the same way, but /sounds/ covers every sound.
func (p *Provider) List(params provider.ListParams) (*provider.ListResult, error) {
	page := params.Page
	if page < 1 {
		page = 1
	}

	search := strings.TrimSpace(params.Search)

	pageSize := browsePageSize
	if search != "" {
		pageSize = searchPageSize
	}

	var listURL string
	switch {
	case search != "" && page == 1:
		listURL = p.baseURL() + "/?s=" + url.QueryEscape(search)
	case search != "":
		listURL = p.baseURL() + "/page/" + strconv.Itoa(page) + "/?s=" + url.QueryEscape(search)
	case page == 1:
		listURL = p.baseURL() + "/sounds/"
	default:
		listURL = p.baseURL() + "/sounds/page/" + strconv.Itoa(page) + "/"
	}

	res, err := p.client().Get(listURL)
	if err != nil {
		return nil, fmt.Errorf("soundboardguy: %w: %v", provider.ErrUpstreamUnavailable, err)
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		slog.Debug("upstream 404, returning an empty page", "provider", key, "url", listURL)
		return emptyPage(page, pageSize), nil
	default:
		return nil, fmt.Errorf("soundboardguy: %w: status %d", provider.ErrBadUpstreamStatus, res.StatusCode)
	}

	return parseList(res.Body, page, pageSize)
}

func emptyPage(page, pageSize int) *provider.ListResult {
	return &provider.ListResult{
		Instants: []provider.Instant{},
		Pages:    provider.InferPages(page, 0, pageSize),
	}
}

func parseList(r io.Reader, page, pageSize int) (*provider.ListResult, error) {
	document, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}

	sources := make(map[string]string)
	document.Find("audio[id] source[src]").Each(func(i int, s *goquery.Selection) {
		id, hasID := s.Parent().Attr("id")
		src, hasSrc := s.Attr("src")
		if hasID && hasSrc {
			sources[id] = src
		}
	})

	instants := []provider.Instant{}
	malformed := 0

	document.Find("div.sbg-big-grid").Each(func(i int, grid *goquery.Selection) {
		if grid.HasClass("--infinite") {
			return
		}

		grid.Find("a.shareable--trigger[data-name][data-audio]").Each(func(j int, a *goquery.Selection) {
			name, hasName := a.Attr("data-name")
			audioID, hasAudio := a.Attr("data-audio")
			src, hasSrc := sources[audioID]
			if !hasName || !hasAudio || name == "" || audioID == "" || !hasSrc || src == "" {
				malformed++
				return
			}

			instants = append(instants, provider.Instant{Name: name, URL: src})
		})
	})

	if malformed > 0 {
		return nil, fmt.Errorf("soundboardguy: %w: %d trigger(s) with no matching audio source", provider.ErrUnexpectedMarkup, malformed)
	}

	return &provider.ListResult{
		Instants: instants,
		Pages:    provider.InferPages(page, len(instants), pageSize),
	}, nil
}
