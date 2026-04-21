package marvin

import (
	"context"
	"fmt"
	"strings"

	"github.com/mooneeb/amgi/internal/config"
)

// cacheEntry stores a resolved Marvin ID alongside its original-case title.
// The original title is kept so error messages can report what a user would
// see in Marvin's UI, not the lowercased lookup key.
type cacheEntry struct {
	id    string
	title string
}

// ListNotFoundError is returned when a list_name cannot be resolved to a
// Marvin category. Available titles are in their original case.
type ListNotFoundError struct {
	Name      string
	Available []string
}

func (e *ListNotFoundError) Error() string {
	return fmt.Sprintf("marvin list %q not found; available: %v", e.Name, e.Available)
}

// LabelNotFoundError is returned when a label_name cannot be resolved to a
// Marvin label. Available titles are in their original case.
type LabelNotFoundError struct {
	Name      string
	Available []string
}

func (e *LabelNotFoundError) Error() string {
	return fmt.Sprintf("marvin label %q not found; available: %v", e.Name, e.Available)
}

// Initialize fetches Marvin categories and labels into the in-memory caches
// and validates that every list_name / label_names reference in the config
// resolves to a real Marvin ID. Called once at startup. Returns a
// ListNotFoundError or LabelNotFoundError if any reference fails to resolve.
func (m *marvin) Initialize(ctx context.Context, cfg *config.Config) error {
	if err := m.populateCaches(ctx); err != nil {
		return fmt.Errorf("populate marvin caches: %w", err)
	}
	for _, mc := range cfg.Marvin.Configs {
		if _, err := m.resolveList(ctx, mc.ListName); err != nil {
			return fmt.Errorf("marvin config %q: %w", mc.ID, err)
		}
		if _, err := m.resolveLabels(ctx, mc.LabelNames); err != nil {
			return fmt.Errorf("marvin config %q: %w", mc.ID, err)
		}
	}
	return nil
}

// populateCaches fetches all categories and labels from Marvin and replaces
// the in-memory caches.
func (m *marvin) populateCaches(ctx context.Context) error {
	if err := m.refreshCategoriesCache(ctx); err != nil {
		return fmt.Errorf("refresh categories cache: %w", err)
	}
	if err := m.refreshLabelsCache(ctx); err != nil {
		return fmt.Errorf("refresh labels cache: %w", err)
	}
	return nil
}

// refreshCategoriesCache fetches categories from Marvin and replaces the cache.
// Case-insensitive keys (all titles lowercased). Duplicate titles — which
// Marvin should not allow — are logged and deduplicated (first match wins).
func (m *marvin) refreshCategoriesCache(ctx context.Context) error {
	cats, err := m.listCategories(ctx)
	if err != nil {
		return err
	}

	cache := make(map[string]cacheEntry, len(cats))
	for _, c := range cats {
		key := strings.ToLower(c.Title)
		if existing, dup := cache[key]; dup {
			m.logger.Warn("duplicate category title in Marvin; keeping first",
				"title", c.Title, "keeping_id", existing.id, "skipping_id", c.ID)
			continue
		}
		cache[key] = cacheEntry{id: c.ID, title: c.Title}
	}

	m.cacheMu.Lock()
	m.categoriesCache = cache
	m.cacheMu.Unlock()
	return nil
}

// refreshLabelsCache fetches labels from Marvin and replaces the cache.
// Same dedupe semantics as categories.
func (m *marvin) refreshLabelsCache(ctx context.Context) error {
	lbls, err := m.listLabels(ctx)
	if err != nil {
		return err
	}

	cache := make(map[string]cacheEntry, len(lbls))
	for _, l := range lbls {
		key := strings.ToLower(l.Title)
		if existing, dup := cache[key]; dup {
			m.logger.Warn("duplicate label title in Marvin; keeping first",
				"title", l.Title, "keeping_id", existing.id, "skipping_id", l.ID)
			continue
		}
		cache[key] = cacheEntry{id: l.ID, title: l.Title}
	}

	m.cacheMu.Lock()
	m.labelsCache = cache
	m.cacheMu.Unlock()
	return nil
}

// resolveList returns the Marvin category _id for a given name. Empty name
// returns empty string (caller omits parentId from addTask body → Marvin
// default places the task in Inbox). On cache miss, refreshes once before
// returning ListNotFoundError.
func (m *marvin) resolveList(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", nil
	}
	key := strings.ToLower(name)

	m.cacheMu.RLock()
	entry, ok := m.categoriesCache[key]
	m.cacheMu.RUnlock()
	if ok {
		return entry.id, nil
	}

	// cache miss — refresh once before erroring
	if err := m.refreshCategoriesCache(ctx); err != nil {
		return "", fmt.Errorf("refresh categories on miss: %w", err)
	}

	m.cacheMu.RLock()
	entry, ok = m.categoriesCache[key]
	available := availableTitles(m.categoriesCache)
	m.cacheMu.RUnlock()
	if !ok {
		return "", &ListNotFoundError{Name: name, Available: available}
	}
	return entry.id, nil
}

// resolveLabels returns a slice of Marvin label _ids for the given names.
// Empty slice returns nil. Case-insensitive exact match per name. On the
// first cache miss, refreshes once; subsequent misses during the same call
// accumulate into the not-found error.
func (m *marvin) resolveLabels(ctx context.Context, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(names))
	var missing []string
	refreshed := false

	for _, name := range names {
		if name == "" {
			continue
		}
		key := strings.ToLower(name)

		m.cacheMu.RLock()
		entry, ok := m.labelsCache[key]
		m.cacheMu.RUnlock()

		if !ok && !refreshed {
			if err := m.refreshLabelsCache(ctx); err != nil {
				return nil, fmt.Errorf("refresh labels on miss: %w", err)
			}
			refreshed = true
			m.cacheMu.RLock()
			entry, ok = m.labelsCache[key]
			m.cacheMu.RUnlock()
		}

		if !ok {
			missing = append(missing, name)
			continue
		}
		ids = append(ids, entry.id)
	}

	if len(missing) > 0 {
		m.cacheMu.RLock()
		available := availableTitles(m.labelsCache)
		m.cacheMu.RUnlock()
		// Report the first missing name; Available carries the full menu for
		// the user to compare against.
		return nil, &LabelNotFoundError{Name: missing[0], Available: available}
	}
	return ids, nil
}

// availableTitles extracts original-case titles from a cache for use in
// not-found error messages.
func availableTitles(cache map[string]cacheEntry) []string {
	out := make([]string, 0, len(cache))
	for _, e := range cache {
		out = append(out, e.title)
	}
	return out
}
