// Package provider abstracts myinstants.com and the other clip sites behind
// one interface, so pkg/server can drive any of them through the same
// handler instead of hand-rolling a fetch/parse pipeline per site.
package provider

import "errors"

// ListParams is provider-agnostic; a provider that doesn't understand a
// field (e.g. Region, which only myinstants honors) ignores it rather than
// erroring, so switching providers from a client that hasn't cleared every
// field doesn't turn into a 400.
type ListParams struct {
	Page   int
	Search string
	Region string
}

// Instant is one playable clip as a provider's listing exposes it.
type Instant struct {
	Name string
	URL  string
}

type ListResult struct {
	Instants []Instant
	Pages    int
}

// Provider fetches and parses one clip site's listing pages. Each
// implementation owns its own URL building, HTTP fetch and markup parsing —
// pkg/server only drives the interface.
type Provider interface {
	Key() string
	DisplayName() string
	List(params ListParams) (*ListResult, error)

	// AllowedContentHosts lists the hosts a clip URL from this provider can
	// legitimately live on, so a content-fetch endpoint can validate a URL
	// before downloading it.
	AllowedContentHosts() []string
}

var (
	// ErrInvalidRegion is returned by a provider whose Region format/value
	// isn't one it can serve — currently only myinstants has a concept of
	// region.
	ErrInvalidRegion = errors.New("provider: region not supported")

	// ErrUnexpectedMarkup means the response parsed but its shape didn't
	// match what the provider's scraper expects (e.g. a listing's names and
	// play links no longer line up) — the site's markup likely changed.
	ErrUnexpectedMarkup = errors.New("provider: unexpected markup shape")

	// ErrUpstreamUnavailable means the request to the provider's site never
	// got a response at all (DNS, connection, timeout).
	ErrUpstreamUnavailable = errors.New("provider: upstream request failed")

	// ErrBadUpstreamStatus means the provider's site answered, but with a
	// status this provider doesn't treat as success or as an empty result.
	ErrBadUpstreamStatus = errors.New("provider: upstream answered with an error status")
)

// Registry looks providers up by their Key().
type Registry map[string]Provider

func (r Registry) Get(key string) (Provider, bool) {
	p, ok := r[key]
	return p, ok
}

// InferPages estimates how many pages of a listing exist from how many
// items this page returned, since none of the sites this package scrapes
// publish a real page count: a full page (pageSize items) means there may
// be a page+1, a short page means this is the last one, and an empty page
// means the previous page was actually the last.
func InferPages(page, count, pageSize int) int {
	switch {
	case count >= pageSize:
		return page + 1
	case count > 0:
		return page
	default:
		return max(1, page-1)
	}
}
