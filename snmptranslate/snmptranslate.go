// Package snmptranslate provides MIB-based OID translation functionality.
//
// This package offers efficient OID-to-name translation by parsing MIB files
// and building optimized data structures for fast lookups. It supports:
//   - Dynamic MIB file loading from directories
//   - Efficient OID lookup using trie data structures
//   - Lazy loading and caching for performance
//   - Batch translation operations
//   - Comprehensive error handling and statistics
//
// Basic Usage:
//
//	translator := snmptranslate.New()
//	if err := translator.Init("/usr/share/snmp/mibs"); err != nil {
//		log.Fatal(err)
//	}
//
//	name, err := translator.Translate(".1.3.6.1.6.3.1.1.5.1")
//	if err != nil {
//		log.Printf("Translation failed: %v", err)
//	} else {
//		log.Printf("OID translates to: %s", name) // "coldStart"
//	}
//
// Performance Features:
//   - Trie-based OID storage for O(log n) lookups
//   - LRU cache for frequently accessed translations
//   - Lazy loading of MIB files to reduce startup time
//   - Batch operations for processing multiple OIDs efficiently
//
// The package is designed to handle large MIB libraries with hundreds or
// thousands of files while maintaining fast lookup performance and reasonable
// memory usage.
package snmptranslate

import (
	"container/list"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Translator provides MIB-based OID translation functionality.
type Translator interface {
	// Init initializes the translator with MIB files from the specified directory
	Init(mibDir string) error

	// Translate converts an OID string to its human-readable name
	Translate(oid string) (string, error)

	// TranslateBatch translates multiple OIDs efficiently
	TranslateBatch(oids []string) (map[string]string, error)

	// LoadMIB loads a specific MIB file
	LoadMIB(filename string) error

	// GetStats returns translation statistics
	GetStats() Stats

	// Close releases resources and cleans up
	Close() error
}

// Stats provides statistics about the translator's performance and state.
type Stats struct {
	LoadedMIBs       int           `json:"loaded_mibs"`
	TotalOIDs        int           `json:"total_oids"`
	CacheHits        int64         `json:"cache_hits"`
	CacheMisses      int64         `json:"cache_misses"`
	TranslationCount int64         `json:"translation_count"`
	AverageLatency   time.Duration `json:"average_latency"`
	MemoryUsage      int64         `json:"memory_usage_bytes"`
}

// OIDEntry represents a single OID mapping in the MIB.
type OIDEntry struct {
	OID         string `json:"oid"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
	Module      string `json:"module,omitempty"`
}

// =============================================================================
// OID Trie Implementation
// =============================================================================

// OIDTrie implements a trie (prefix tree) data structure optimized for OID storage and lookup.
// It provides O(log n) insertion and lookup operations where n is the depth of the OID tree.
//
// The trie structure mirrors the hierarchical nature of SNMP OIDs, where each node
// represents a numeric component of the OID path. This allows for efficient prefix
// matching and hierarchical lookups.
//
// Example OID structure in trie:
//
//	.1.3.6.1.6.3.1.1.5.1 -> "coldStart"
//	.1.3.6.1.6.3.1.1.5.2 -> "warmStart"
//
// Would create a trie structure:
//
//	1 -> 3 -> 6 -> 1 -> 6 -> 3 -> 1 -> 1 -> 5 -> 1 (name: "coldStart")
//	                                         -> 2 (name: "warmStart")
type OIDTrie struct {
	mu   sync.RWMutex
	root *TrieNode
	size int
}

// TrieNode represents a single node in the OID trie.
type TrieNode struct {
	// children maps numeric OID components to child nodes
	children map[int]*TrieNode

	// name is the human-readable name for this OID (only set for leaf nodes)
	name string

	// isLeaf indicates if this node represents a complete OID
	isLeaf bool

	// depth tracks the depth of this node in the trie (for debugging/stats)
	depth int
}

// =============================================================================
// Cache Implementation
// =============================================================================

// Cache implements a thread-safe LRU (Least Recently Used) cache for OID translations.
// It provides fast access to frequently requested OID translations while automatically
// evicting least recently used entries when the cache reaches its maximum size.
//
// The cache uses a combination of a hash map for O(1) lookups and a doubly-linked list
// for O(1) LRU operations. This ensures that both cache hits and cache maintenance
// operations are highly efficient.
type Cache struct {
	mu       sync.RWMutex
	capacity int
	items    map[string]*list.Element
	lruList  *list.List
	stats    CacheStats
}

// CacheEntry represents a single entry in the cache.
type CacheEntry struct {
	key       string
	value     string
	timestamp time.Time
	hitCount  int64
}

// CacheStats provides statistics about cache performance.
type CacheStats struct {
	Hits         int64     `json:"hits"`
	Misses       int64     `json:"misses"`
	Evictions    int64     `json:"evictions"`
	Size         int       `json:"size"`
	Capacity     int       `json:"capacity"`
	HitRatio     float64   `json:"hit_ratio"`
	LastAccess   time.Time `json:"last_access"`
	LastEviction time.Time `json:"last_eviction"`
}

// =============================================================================
// MIB Parser Implementation
// =============================================================================

// MIBParser handles parsing of MIB files to extract OID-to-name mappings.
// It supports basic MIB syntax including OBJECT-TYPE and NOTIFICATION-TYPE definitions.
//
// The parser uses regular expressions to identify and extract key information
// from MIB files, focusing on the most common constructs needed for OID translation.
//
// Supported MIB constructs:
//   - OBJECT-TYPE definitions with OID assignments
//   - NOTIFICATION-TYPE definitions
//   - Basic MODULE-IDENTITY parsing
//   - Simple OID assignments (name OBJECT IDENTIFIER ::= { parent child })
//
// The parser is designed to be robust and handle variations in MIB file formatting
// while extracting the essential OID-to-name mappings needed for translation.
type MIBParser struct {
	// Regular expressions for parsing different MIB constructs
	objectTypeRegex     *regexp.Regexp
	notificationRegex   *regexp.Regexp
	oidAssignmentRegex  *regexp.Regexp
	moduleIdentityRegex *regexp.Regexp

	// Context tracking for multi-line parsing
	currentModule string
	oidContext    map[string]string // Maps symbolic names to OID strings
}

// translator implements the Translator interface.
type translator struct {
	mu           sync.RWMutex
	mibDir       string
	trie         *OIDTrie
	cache        *Cache
	loadedMIBs   map[string]bool
	stats        Stats
	initialized  bool
	lazyLoading  bool
	maxCacheSize int
}

// Config holds configuration options for the translator.
type Config struct {
	LazyLoading  bool `json:"lazy_loading"`
	MaxCacheSize int  `json:"max_cache_size"`
	EnableStats  bool `json:"enable_stats"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		LazyLoading:  true,
		MaxCacheSize: 10000,
		EnableStats:  true,
	}
}

// New creates a new translator with default configuration.
func New() Translator {
	return NewWithConfig(DefaultConfig())
}

// NewWithConfig creates a new translator with the specified configuration.
func NewWithConfig(config Config) Translator {
	return &translator{
		trie:         NewOIDTrie(),
		cache:        NewCache(config.MaxCacheSize),
		loadedMIBs:   make(map[string]bool),
		lazyLoading:  config.LazyLoading,
		maxCacheSize: config.MaxCacheSize,
	}
}

// Init initializes the translator with MIB files from the specified directory.
func (t *translator) Init(mibDir string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.initialized {
		return fmt.Errorf("translator already initialized")
	}

	// Validate MIB directory
	if _, err := os.Stat(mibDir); os.IsNotExist(err) {
		return fmt.Errorf("MIB directory does not exist: %s", mibDir)
	}

	t.mibDir = mibDir

	if t.lazyLoading {
		// In lazy loading mode, just mark as initialized
		t.initialized = true
		return nil
	}

	// Load all MIB files immediately
	return t.loadAllMIBs()
}

// Translate converts an OID string to its human-readable name.
func (t *translator) Translate(oid string) (string, error) {
	if !t.initialized {
		return "", fmt.Errorf("translator not initialized")
	}

	start := time.Now()
	defer func() {
		t.updateStats(time.Since(start))
	}()

	// Normalize OID
	normalizedOID := normalizeOID(oid)

	// Check cache first
	if cached, found := t.cache.Get(normalizedOID); found {
		t.mu.Lock()
		t.stats.CacheHits++
		t.mu.Unlock()
		return cached, nil
	}

	t.mu.Lock()
	t.stats.CacheMisses++
	t.mu.Unlock()

	// Try to find in trie
	t.mu.RLock()
	name := t.trie.Lookup(normalizedOID)
	t.mu.RUnlock()

	if name != "" {
		// Cache the result
		t.cache.Set(normalizedOID, name)
		return name, nil
	}

	// If lazy loading is enabled and OID not found, try loading more MIBs
	if t.lazyLoading {
		if err := t.loadMIBsOnDemand(); err == nil {
			t.mu.RLock()
			name = t.trie.Lookup(normalizedOID)
			t.mu.RUnlock()

			if name != "" {
				t.cache.Set(normalizedOID, name)
				return name, nil
			}
		}
	}

	// Fallback to basic translation for common OIDs
	if basicName := translateCommonOID(normalizedOID); basicName != normalizedOID {
		t.cache.Set(normalizedOID, basicName)
		return basicName, nil
	}

	return normalizedOID, fmt.Errorf("OID not found: %s", normalizedOID)
}

// TranslateBatch translates multiple OIDs efficiently.
func (t *translator) TranslateBatch(oids []string) (map[string]string, error) {
	if !t.initialized {
		return nil, fmt.Errorf("translator not initialized")
	}

	result := make(map[string]string, len(oids))
	var errors []string

	for _, oid := range oids {
		name, err := t.Translate(oid)
		result[oid] = name // Always include in result, even if translation failed

		if err != nil {
			errors = append(errors, fmt.Sprintf("OID %s: %v", oid, err))
		}
	}

	if len(errors) > 0 {
		return result, fmt.Errorf("batch translation errors: %s", strings.Join(errors, "; "))
	}

	return result, nil
}

// LoadMIB loads a specific MIB file.
func (t *translator) LoadMIB(filename string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.loadedMIBs[filename] {
		return nil // Already loaded
	}

	parser := NewMIBParser()
	entries, err := parser.ParseFile(filename)
	if err != nil {
		return fmt.Errorf("failed to parse MIB file %s: %w", filename, err)
	}

	// Add entries to trie
	for _, entry := range entries {
		t.trie.Insert(entry.OID, entry.Name)
	}

	t.loadedMIBs[filename] = true
	t.stats.LoadedMIBs++
	t.stats.TotalOIDs += len(entries)

	return nil
}

// GetStats returns translation statistics.
func (t *translator) GetStats() Stats {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.stats
}

// Close releases resources and cleans up.
func (t *translator) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.cache.Clear()
	t.trie = NewOIDTrie()
	t.loadedMIBs = make(map[string]bool)
	t.initialized = false

	return nil
}

// loadAllMIBs loads all MIB files from the configured directory.
func (t *translator) loadAllMIBs() error {
	return filepath.Walk(t.mibDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && isMIBFile(path) {
			if err := t.loadMIBFile(path); err != nil {
				// Log error but continue with other files
				fmt.Printf("Warning: failed to load MIB file %s: %v\n", path, err)
			}
		}

		return nil
	})
}

// loadMIBFile loads a single MIB file.
func (t *translator) loadMIBFile(filename string) error {
	if t.loadedMIBs[filename] {
		return nil
	}

	parser := NewMIBParser()
	entries, err := parser.ParseFile(filename)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		t.trie.Insert(entry.OID, entry.Name)
	}

	t.loadedMIBs[filename] = true
	t.stats.LoadedMIBs++
	t.stats.TotalOIDs += len(entries)

	return nil
}

// loadMIBsOnDemand attempts to load additional MIB files when an OID is not found.
func (t *translator) loadMIBsOnDemand() error {
	// Get list of unloaded MIB files
	unloadedFiles, err := t.getUnloadedMIBFiles()
	if err != nil {
		return err
	}

	// Try loading a few more MIB files
	loadCount := 0
	maxLoad := 5 // Limit to avoid performance impact

	for _, filename := range unloadedFiles {
		if loadCount >= maxLoad {
			break
		}

		t.mu.Lock()
		err := t.loadMIBFile(filename)
		t.mu.Unlock()

		if err == nil {
			loadCount++
		}
	}

	return nil
}

// getUnloadedMIBFiles returns a list of MIB files that haven't been loaded yet.
func (t *translator) getUnloadedMIBFiles() ([]string, error) {
	var unloaded []string

	err := filepath.Walk(t.mibDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && isMIBFile(path) && !t.loadedMIBs[path] {
			unloaded = append(unloaded, path)
		}

		return nil
	})

	// Sort by filename for consistent loading order
	sort.Strings(unloaded)

	return unloaded, err
}

// updateStats updates translation statistics.
func (t *translator) updateStats(duration time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.stats.TranslationCount++

	// Update average latency using exponential moving average
	if t.stats.AverageLatency == 0 {
		t.stats.AverageLatency = duration
	} else {
		// EMA with alpha = 0.1
		t.stats.AverageLatency = time.Duration(
			0.9*float64(t.stats.AverageLatency) + 0.1*float64(duration),
		)
	}
}

// isMIBFile checks if a file is likely a MIB file based on its extension.
func isMIBFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".mib" || ext == ".txt" || ext == "" // MIB files often have no extension
}

// normalizeOID normalizes an OID string by ensuring it has a leading dot.
func normalizeOID(oid string) string {
	if oid == "" {
		return oid
	}
	if !strings.HasPrefix(oid, ".") {
		return "." + oid
	}
	return oid
}

// translateCommonOID provides fallback translation for common SNMP trap OIDs.
func translateCommonOID(oid string) string {
	switch oid {
	case ".1.3.6.1.6.3.1.1.5.1":
		return "coldStart"
	case ".1.3.6.1.6.3.1.1.5.2":
		return "warmStart"
	case ".1.3.6.1.6.3.1.1.5.3":
		return "linkDown"
	case ".1.3.6.1.6.3.1.1.5.4":
		return "linkUp"
	case ".1.3.6.1.6.3.1.1.5.5":
		return "authenticationFailure"
	case ".1.3.6.1.6.3.1.1.5.6":
		return "egpNeighborLoss"
	default:
		return oid
	}
}
