// Package favorites stores each bot owner's list of favourite instants, one JSON file per owner.
package favorites

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// MaxInstants is the most favourites one list may hold.
const MaxInstants = 1000

// MaxNameLength is the most characters a favourite's name may have.
const MaxNameLength = 200

var (
	// ErrConflict means the list changed since the revision the caller started from.
	ErrConflict = errors.New("favorites changed since the base revision")
	// ErrInvalid means a list broke one of the validation rules.
	ErrInvalid = errors.New("invalid favorites")
	// ErrInvalidOwner means an owner name can't be used as a list key.
	ErrInvalidOwner = errors.New("invalid owner")
)

var (
	ownerPattern = regexp.MustCompile(`^[a-z0-9._]{2,32}$`)
	keyPattern   = regexp.MustCompile(`^[a-z0-9]$`)
)

// Instant is one favourite clip, with the letter or digit that plays it.
type Instant struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Key  string `json:"key,omitempty"`
}

// List is one owner's favourites at a revision.
type List struct {
	Owner     string     `json:"owner"`
	Revision  int64      `json:"revision"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
	Instants  []Instant  `json:"instants"`
}

// OwnerKey normalizes a Discord username into the key its list is stored under.
func OwnerKey(owner string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(owner))
	if !ownerPattern.MatchString(key) || strings.Contains(key, "..") {
		return "", fmt.Errorf("%w: %q is not a Discord username (2-32 of a-z, 0-9, . and _)", ErrInvalidOwner, owner)
	}

	return key, nil
}

// Validate reports the first rule a list of favourites breaks, wrapped in ErrInvalid.
func Validate(instants []Instant) error {
	if len(instants) > MaxInstants {
		return fmt.Errorf("%w: %d items, at most %d", ErrInvalid, len(instants), MaxInstants)
	}

	urls := make(map[string]bool, len(instants))
	keys := make(map[string]bool)
	for i, item := range instants {
		name := strings.TrimSpace(item.Name)
		if name == "" || utf8.RuneCountInString(name) > MaxNameLength {
			return fmt.Errorf("%w: item %d name must be 1-%d characters", ErrInvalid, i, MaxNameLength)
		}

		u, err := url.Parse(item.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("%w: item %d url must be an absolute http or https URL", ErrInvalid, i)
		}
		if urls[item.URL] {
			return fmt.Errorf("%w: item %d url is a duplicate", ErrInvalid, i)
		}
		urls[item.URL] = true

		if item.Key == "" {
			continue
		}
		if !keyPattern.MatchString(item.Key) {
			return fmt.Errorf("%w: item %d key must be one of a-z or 0-9", ErrInvalid, i)
		}
		if keys[item.Key] {
			return fmt.Errorf("%w: item %d key is a duplicate", ErrInvalid, i)
		}
		keys[item.Key] = true
	}

	return nil
}

// Store keeps every owner's list in memory, backed by one file per owner.
type Store struct {
	dir string
	now func() time.Time

	mu    sync.Mutex
	lists map[string]List
}

// NewStore returns a Store whose lists live in dir/favorites.
func NewStore(dir string) *Store {
	return &Store{
		dir:   filepath.Join(dir, "favorites"),
		now:   time.Now,
		lists: make(map[string]List),
	}
}

// Get returns the owner's list, or an empty one at revision 0 if none was ever stored.
func (s *Store) Get(owner string) (List, error) {
	key, err := OwnerKey(owner)
	if err != nil {
		return List{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.load(key)
}

// Put replaces the owner's list if base is its current revision, and returns the new list.
func (s *Store) Put(owner string, base int64, instants []Instant) (List, error) {
	key, err := OwnerKey(owner)
	if err != nil {
		return List{}, err
	}

	if err := Validate(instants); err != nil {
		return List{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.load(key)
	if err != nil {
		return List{}, err
	}

	if base != current.Revision {
		return List{}, ErrConflict
	}

	now := s.now().UTC().Truncate(time.Second)
	next := List{
		Owner:     key,
		Revision:  current.Revision + 1,
		UpdatedAt: &now,
		Instants:  append([]Instant{}, instants...),
	}

	if err := s.write(key, next); err != nil {
		return List{}, err
	}

	s.lists[key] = next

	return next, nil
}

func (s *Store) path(key string) string {
	return filepath.Join(s.dir, key+".json")
}

func (s *Store) load(key string) (List, error) {
	if list, ok := s.lists[key]; ok {
		return list, nil
	}

	raw, err := os.ReadFile(s.path(key))
	switch {
	case err == nil:
	case errors.Is(err, os.ErrNotExist):
		list := List{Owner: key, Instants: []Instant{}}
		s.lists[key] = list
		return list, nil
	default:
		return List{}, fmt.Errorf("read favorites: %w", err)
	}

	var list List
	if err := json.Unmarshal(raw, &list); err != nil {
		return List{}, fmt.Errorf("parse favorites %s: %w", s.path(key), err)
	}

	list.Owner = key
	if list.Instants == nil {
		list.Instants = []Instant{}
	}

	s.lists[key] = list

	return list, nil
}

func (s *Store) write(key string, list List) (err error) {
	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("encode favorites: %w", err)
	}

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create favorites dir: %w", err)
	}

	tmp, err := os.CreateTemp(s.dir, "."+key+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create favorites temp file: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()

	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write favorites: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync favorites: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close favorites: %w", err)
	}

	if err := os.Rename(tmp.Name(), s.path(key)); err != nil {
		return fmt.Errorf("replace favorites: %w", err)
	}

	return nil
}
