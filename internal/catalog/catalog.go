// Package catalog fetches and caches the api2convert conversions catalog (the
// supported categories, targets and their option schemas), so discovery and
// completion are fast and tolerant of transient offline moments.
package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	api2convert "github.com/QaamGo/api2convert-go"
)

// Conversion is one supported conversion in the catalog.
type Conversion struct {
	ID       string         `json:"id"`
	Category string         `json:"category"`
	Target   string         `json:"target"`
	Options  map[string]any `json:"options,omitempty"`
}

// Catalog is the cached set of supported conversions.
type Catalog struct {
	FetchedAt   time.Time    `json:"fetched_at"`
	Conversions []Conversion `json:"conversions"`
}

const ttl = 24 * time.Hour

// cachePath keys the cache file by a hash of cacheKey (the API key + base URL),
// so one account's catalog is never served to another.
func cachePath(cacheKey string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(cacheKey))
	name := "catalog-" + hex.EncodeToString(sum[:])[:12] + ".json"
	return filepath.Join(base, "api2convert", name), nil
}

// Load returns the catalog, using a fresh cached copy when available (unless
// refresh is set). On a fetch error it falls back to any cached copy, even stale.
// cacheKey scopes the on-disk cache to an account (e.g. apiKey+"|"+baseURL).
func Load(ctx context.Context, c *api2convert.Client, refresh bool, cacheKey string) (*Catalog, error) {
	if !refresh {
		if cat, ok := loadCache(cacheKey, false); ok {
			return cat, nil
		}
	}
	cat, err := fetch(ctx, c)
	if err != nil {
		if cat, ok := loadCache(cacheKey, true); ok {
			return cat, nil
		}
		return nil, err
	}
	saveCache(cacheKey, cat)
	return cat, nil
}

func fetch(ctx context.Context, c *api2convert.Client) (*Catalog, error) {
	var all []Conversion
	for page := 1; page <= 200; page++ {
		rows, err := c.Conversions().List(ctx, "", "", page)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			all = append(all, Conversion{
				ID:       asString(r["id"]),
				Category: asString(r["category"]),
				Target:   asString(r["target"]),
				Options:  asObject(r["options"]),
			})
		}
	}
	return &Catalog{FetchedAt: time.Now().UTC(), Conversions: all}, nil
}

func loadCache(cacheKey string, allowStale bool) (*Catalog, bool) {
	p, err := cachePath(cacheKey)
	if err != nil {
		return nil, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	var cat Catalog
	if err := json.Unmarshal(b, &cat); err != nil {
		return nil, false
	}
	if !allowStale && time.Since(cat.FetchedAt) > ttl {
		return nil, false
	}
	return &cat, true
}

func saveCache(cacheKey string, cat *Catalog) {
	p, err := cachePath(cacheKey)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	b, err := json.Marshal(cat)
	if err != nil {
		return
	}
	_ = os.WriteFile(p, b, 0o644)
}

// Categories returns the sorted, unique category names.
func (c *Catalog) Categories() []string {
	set := map[string]struct{}{}
	for _, cv := range c.Conversions {
		if cv.Category != "" {
			set[cv.Category] = struct{}{}
		}
	}
	return sortedKeys(set)
}

// Targets returns the sorted, unique target format codes.
func (c *Catalog) Targets() []string {
	set := map[string]struct{}{}
	for _, cv := range c.Conversions {
		if cv.Target != "" {
			set[cv.Target] = struct{}{}
		}
	}
	return sortedKeys(set)
}

// Filter returns conversions in a category (empty = all), sorted by target.
func (c *Catalog) Filter(category string) []Conversion {
	var out []Conversion
	for _, cv := range c.Conversions {
		if category == "" || strings.EqualFold(cv.Category, category) {
			out = append(out, cv)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Target < out[j].Target
	})
	return out
}

// Search returns conversions whose target or category contains q (case-insensitive).
func (c *Catalog) Search(q string) []Conversion {
	q = strings.ToLower(q)
	var out []Conversion
	seen := map[string]struct{}{}
	for _, cv := range c.Conversions {
		if strings.Contains(strings.ToLower(cv.Target), q) || strings.Contains(strings.ToLower(cv.Category), q) {
			key := cv.Category + "/" + cv.Target
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, cv)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Target < out[j].Target })
	return out
}

// HasTarget reports whether the catalog contains the given target.
func (c *Catalog) HasTarget(target string) bool {
	for _, cv := range c.Conversions {
		if strings.EqualFold(cv.Target, target) {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asObject(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}
