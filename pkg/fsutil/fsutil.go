package fsutil

import (
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/httpclient"
)

var (
	ErrNotFound              = errors.New("resource not found")
	ErrUnsuportedAudioFormat = errors.New("unduported audio format")
	ErrUpstreamUnavailable   = errors.New("upstream request failed")
	ErrHostNotAllowed        = errors.New("host not allowed")
)

// Cache resolves instant links to files on disk, downloading them on first
// use. Client and Dir default to production values when unset. Only hosts in
// AllowedHosts are downloaded from; with none set, nothing is.
type Cache struct {
	Client       *http.Client
	Dir          string
	AllowedHosts []string
}

// Default backs the package-level functions the rest of the app calls.
var Default = &Cache{}

var defaultClient = httpclient.New()

func GetFromCache(link string) (*os.File, error) {
	return Default.Get(link)
}

func GetCacheDirOrCreate() (string, error) {
	return Default.DirOrCreate()
}

func (c *Cache) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}

	return defaultClient
}

func (c *Cache) Get(link string) (*os.File, error) {
	cdr, err := c.DirOrCreate()
	if err != nil {
		return nil, fmt.Errorf("failed to get cache dir: %w", err)
	}

	fname := filepath.Join(cdr, fmt.Sprintf("%x.mp3", md5.Sum(([]byte(link)))))
	ifile, err := os.Open(fname)
	switch {
	case err == nil:
		slog.Debug("cache hit", "link", link, "path", fname)
		return ifile, nil
	case os.IsNotExist(err):
		// continue
	default:
		return nil, err
	}

	target, err := c.downloadURL(link)
	if err != nil {
		slog.Debug("download rejected", "reason", "host-not-allowed", "link", link)
		return nil, err
	}

	slog.Debug("cache miss, downloading", "link", link, "path", fname)
	start := time.Now()

	fr, err := c.client().Get(target)
	if err != nil {
		return nil, err
	}
	defer fr.Body.Close()

	switch fr.StatusCode {
	case http.StatusOK:
		// continue
	case http.StatusNotFound:
		slog.Debug("download rejected", "link", link, "reason", "status", "status", fr.StatusCode)
		return nil, ErrNotFound
	default:
		slog.Debug("download rejected", "link", link, "reason", "status", "status", fr.StatusCode)
		return nil, fmt.Errorf("%w: status %d", ErrUpstreamUnavailable, fr.StatusCode)
	}

	// Sniffing consumes from fr.Body without putting it back, so the head is
	// read here and stitched back on before copying the rest.
	head := make([]byte, 8192)
	n, err := io.ReadFull(fr.Body, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("failed to read instant: %w", err)
	}
	head = head[:n]

	if !looksLikeMP3(head) {
		slog.Debug("download rejected", "link", link, "reason", "not-mp3")
		return nil, ErrUnsuportedAudioFormat
	}

	file, err := os.Create(fname)
	if err != nil {
		return nil, err
	}

	written, err := io.Copy(file, io.MultiReader(bytes.NewReader(head), fr.Body))
	if err != nil {
		return nil, err
	}

	slog.Debug("download finished", "link", link, "bytes", written, "durationMs", time.Since(start).Milliseconds())

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	return file, nil
}

func (c *Cache) downloadURL(link string) (string, error) {
	u, err := url.Parse(link)
	if err != nil {
		return "", ErrHostNotAllowed
	}

	var scheme string
	switch u.Scheme {
	case "https":
		scheme = "https"
	case "http":
		scheme = "http"
	default:
		return "", ErrHostNotAllowed
	}

	var host string
	for _, allowed := range c.AllowedHosts {
		if strings.EqualFold(allowed, u.Hostname()) {
			host = allowed
			break
		}
	}
	if host == "" {
		return "", ErrHostNotAllowed
	}

	if p := u.Port(); p != "" {
		port, err := strconv.Atoi(p)
		if err != nil {
			return "", ErrHostNotAllowed
		}
		host = net.JoinHostPort(host, strconv.Itoa(port))
	}

	safe := url.URL{Scheme: scheme, Host: host, Path: u.Path, RawPath: u.RawPath, RawQuery: u.RawQuery}
	return safe.String(), nil
}

func looksLikeMP3(head []byte) bool {
	if len(head) >= 3 && string(head[:3]) == "ID3" {
		return true
	}
	return len(head) >= 2 && head[0] == 0xFF && head[1]&0xE0 == 0xE0
}

func (c *Cache) DirOrCreate() (string, error) {
	cdr := c.Dir
	if cdr == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}

		cdr = filepath.Join(h, ".instants")
	}

	if err := os.MkdirAll(cdr, os.ModePerm); err != nil {
		return "", err
	}

	return cdr, nil
}
