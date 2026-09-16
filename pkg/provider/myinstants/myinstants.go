// Package myinstants scrapes myinstants.com's listing pages.
package myinstants

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/httpclient"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/provider"
)

const (
	key         = "myinstants"
	displayName = "MyInstants"

	pageSize      = 36
	defaultRegion = "us"
)

var (
	playURLPattern = regexp.MustCompile(`play\(\s*'([^']+)'`)
	regionPattern  = regexp.MustCompile(`^[a-z]{2}$`)
)

var defaultClient = httpclient.New()

// Provider scrapes myinstants.com. BaseURL and Client default to production
// values when unset, so a test can point both at an httptest server.
type Provider struct {
	BaseURL string
	Client  *http.Client
}

func New() *Provider { return &Provider{} }

func (p *Provider) Key() string         { return key }
func (p *Provider) DisplayName() string { return displayName }

func (p *Provider) AllowedContentHosts() []string {
	return []string{"www.myinstants.com"}
}

func (p *Provider) baseURL() string {
	if p.BaseURL != "" {
		return p.BaseURL
	}

	return "https://www.myinstants.com"
}

func (p *Provider) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}

	return defaultClient
}

func (p *Provider) List(params provider.ListParams) (*provider.ListResult, error) {
	region := strings.ToLower(strings.TrimSpace(params.Region))
	if region == "" {
		region = defaultRegion
	}
	if !regionPattern.MatchString(region) {
		return nil, provider.ErrInvalidRegion
	}

	page := params.Page
	if page < 1 {
		page = 1
	}

	var listURL string
	search := strings.Replace(strings.TrimSpace(params.Search), " ", "+", -1)
	if search != "" {
		listURL = p.baseURL() + "/search/?page=" + strconv.Itoa(page) + "&name=" + search
	} else {
		listURL = p.baseURL() + "/en/index/" + region + "/?page=" + strconv.Itoa(page)
	}

	res, err := p.client().Get(listURL)
	if err != nil {
		return nil, fmt.Errorf("myinstants: %w: %v", provider.ErrUpstreamUnavailable, err)
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return &provider.ListResult{
			Instants: []provider.Instant{},
			Pages:    provider.InferPages(page, 0, pageSize),
		}, nil
	default:
		return nil, fmt.Errorf("myinstants: %w: status %d", provider.ErrBadUpstreamStatus, res.StatusCode)
	}

	return parseList(res.Body, p.baseURL(), page)
}

func parseList(r io.Reader, baseURL string, page int) (*provider.ListResult, error) {
	document, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}

	names := []string{}
	links := []string{}

	document.Find(".instant-link").Each(func(i int, anchor *goquery.Selection) {
		names = append(names, anchor.Text())
	})

	document.Find(".small-button").Each(func(i int, button *goquery.Selection) {
		onclick, ok := button.Attr("onclick")
		if !ok {
			return
		}

		match := playURLPattern.FindStringSubmatch(onclick)
		if match == nil {
			return
		}

		links = append(links, baseURL+match[1])
	})

	if len(names) != len(links) {
		return nil, fmt.Errorf("myinstants: %w: %d names, %d links", provider.ErrUnexpectedMarkup, len(names), len(links))
	}

	instants := []provider.Instant{}
	for i, name := range names {
		instants = append(instants, provider.Instant{
			Name: name,
			URL:  links[i],
		})
	}

	return &provider.ListResult{
		Instants: instants,
		Pages:    provider.InferPages(page, len(instants), pageSize),
	}, nil
}
