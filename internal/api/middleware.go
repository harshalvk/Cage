package api

import (
	"context"
	"time"

	"github.com/harshalvk/cage/internal/cache"
	"github.com/harshalvk/cage/internal/store"
)

const apiKeyCacheTTL = 5 * time.Minute

func validateWithCache(ctx context.Context, st *store.Store, c *cache.Cache, keyHash string) (bool, error) {
	cacheKey := "apikey:" + keyHash

	// checkng if it is cached
	cached, err := c.Get(ctx, cacheKey)
	if err == nil && cached != "" {
		return cached == "valid", nil
	}

	// cache miss
	valid, err := st.ValidateAPIKey(ctx, keyHash)
	if err != nil {
		return false, err
	}

	// add cache for next time; cache both valid AND invlaid results -
	// this also protects the db from repeated lookups of a garbage/malicious key
	result := "invalid"
	if valid {
		result = "valid"
	}
	_ = c.Set(ctx, cacheKey, result, apiKeyCacheTTL) // ignore cache-write errors - caching is best-effor

	return valid, nil
}
