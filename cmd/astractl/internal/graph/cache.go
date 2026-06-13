package graph

import (
	"fmt"
	"os"
	"path/filepath"
)

// CacheManager handles caching of dependency graphs.
type CacheManager struct {
	cacheDir string
}

// NewCacheManager creates a new cache manager.
// cacheDir is typically ".astractl" in the project root.
func NewCacheManager(cacheDir string) *CacheManager {
	return &CacheManager{
		cacheDir: cacheDir,
	}
}

// GetCachePath returns the full path to the cache file.
func (cm *CacheManager) GetCachePath() string {
	return filepath.Join(cm.cacheDir, "deps-cache.json")
}

// Load attempts to load a cached graph from disk.
// Returns the cached graph if valid, or nil if cache is invalid or missing.
func (cm *CacheManager) Load(goModPath string) (*Graph, error) {
	cachePath := cm.GetCachePath()

	// Check if cache file exists
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return nil, nil // Cache doesn't exist, not an error
	}

	// Load cached graph
	cached, err := LoadCachedGraphFromFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load cache: %w", err)
	}

	// Compute current go.mod hash
	currentHash, err := ComputeGoModHash(goModPath)
	if err != nil {
		return nil, fmt.Errorf("failed to compute go.mod hash: %w", err)
	}

	// Validate cache
	if !cached.IsValid(currentHash) {
		return nil, nil // Cache is invalid, return nil
	}

	return cached.Graph, nil
}

// Save writes the graph to disk with hash and timestamp.
func (cm *CacheManager) Save(graph *Graph, goModPath string) error {
	// Ensure cache directory exists
	if err := os.MkdirAll(cm.cacheDir, 0700); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Compute go.mod hash
	hash, err := ComputeGoModHash(goModPath)
	if err != nil {
		return fmt.Errorf("failed to compute go.mod hash: %w", err)
	}

	// Create cached graph
	cached := NewCachedGraph(graph, hash)

	// Save to file
	cachePath := cm.GetCachePath()
	if err := cached.SaveToFile(cachePath); err != nil {
		return fmt.Errorf("failed to save cache: %w", err)
	}

	return nil
}

// Clear removes the cache file.
func (cm *CacheManager) Clear() error {
	cachePath := cm.GetCachePath()
	if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear cache: %w", err)
	}
	return nil
}
