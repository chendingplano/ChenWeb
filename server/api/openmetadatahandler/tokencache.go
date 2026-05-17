package openmetadatahandler

import (
	"sync"
	"time"
)

const provisionCacheTTL = time.Hour

type provisionCache struct {
	mu      sync.RWMutex
	entries map[string]time.Time
}

func newProvisionCache() *provisionCache {
	return &provisionCache{entries: make(map[string]time.Time)}
}

func (c *provisionCache) isProvisioned(userID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	exp, ok := c.entries[userID]
	return ok && time.Now().Before(exp)
}

func (c *provisionCache) markProvisioned(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[userID] = time.Now().Add(provisionCacheTTL)
}

var userProvisionCache = newProvisionCache()
