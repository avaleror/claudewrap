// Deduplication cache for compression and paste filter results.
// Keyed by SHA-256 of original text. Persisted to ~/.claudewrap/compress-cache.json
// so identical content is never re-compressed across sessions.
package compress

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type cacheEntry struct {
	Compressed string `json:"compressed"`
	Engine     string `json:"engine"`
}

var (
	cacheMu     sync.RWMutex
	memCache    = map[string]cacheEntry{}
	cacheReady  bool
)

func cacheFilePath() string {
	return filepath.Join(os.Getenv("HOME"), ".claudewrap", "compress-cache.json")
}

func cacheKey(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:16]) // 128-bit prefix — collision-free for this use case
}

func loadCacheOnce() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if cacheReady {
		return
	}
	cacheReady = true
	data, err := os.ReadFile(cacheFilePath())
	if err != nil {
		return
	}
	json.Unmarshal(data, &memCache) //nolint:errcheck — corrupt cache is ignored, not fatal
}

func cacheGet(text string) (cacheEntry, bool) {
	loadCacheOnce()
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	e, ok := memCache[cacheKey(text)]
	return e, ok
}

func cacheSet(text string, e cacheEntry) {
	loadCacheOnce()
	cacheMu.Lock()
	defer cacheMu.Unlock()
	memCache[cacheKey(text)] = e
	data, err := json.Marshal(memCache)
	if err != nil {
		return
	}
	dir := filepath.Join(os.Getenv("HOME"), ".claudewrap")
	os.MkdirAll(dir, 0700)              //nolint:errcheck
	os.WriteFile(cacheFilePath(), data, 0600) //nolint:errcheck
}
