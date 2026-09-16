// Package provider abstracts myinstants.com and the other clip sites behind
// one Provider interface.
package provider

import "errors"

// ListParams is provider-agnostic; a provider ignores a field it doesn't
// understand (e.g. Region) rather than erroring.
type ListParams struct {
	Page   int
	Search string
	Region string
}

// Instant is one playable clip from a provider's listing.
type Instant struct {
	Name string
	URL  string
}

// ListResult is one page of a provider's listing.
type ListResult struct {
	Instants []Instant
	Pages    int
}

// Provider fetches and parses one clip site's listing pages. Each
// implementation owns its own URL building, HTTP fetch and markup parsing.
type Provider interface {
	Key() string
	DisplayName() string
	List(params ListParams) (*ListResult, error)

	// AllowedContentHosts lists the hosts a clip URL from this provider may
	// live on, so a content-fetch endpoint can validate a URL before
	// downloading it.
	AllowedContentHosts() []string

	// SupportsRegion reports whether Region in ListParams does anything for
	// this provider, so a provider picker can decide whether to show a
	// region field at all.
	SupportsRegion() bool
}

var (
	// ErrInvalidRegion is returned when a provider doesn't support the
	// given Region — only myinstants has a concept of region.
	ErrInvalidRegion = errors.New("provider: region not supported")

	// ErrUnexpectedMarkup means the response parsed but didn't have the
	// shape the scraper expects — the site's markup likely changed.
	ErrUnexpectedMarkup = errors.New("provider: unexpected markup shape")

	// ErrUpstreamUnavailable means the request to the provider's site got
	// no response at all.
	ErrUpstreamUnavailable = errors.New("provider: upstream request failed")

	// ErrBadUpstreamStatus means the provider's site answered with a
	// status that isn't success or an empty result.
	ErrBadUpstreamStatus = errors.New("provider: upstream answered with an error status")
)

// Registry looks providers up by their Key().
type Registry map[string]Provider

// Get looks up a provider by key.
func (r Registry) Get(key string) (Provider, bool) {
	p, ok := r[key]
	return p, ok
}

// InferPages estimates how many pages of a listing remain from how many
// items this page returned, since none of these sites publish a real page
// count: a full page means there may be another, a short page means this is
// the last one, an empty page means the previous one was actually last.
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
