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
	"bufio"
	"container/list"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

// =============================================================================
// OID Trie Constructor and Methods
// =============================================================================

// NewOIDTrie creates a new empty OID trie.
func NewOIDTrie() *OIDTrie {
	return &OIDTrie{
		root: &TrieNode{
			children: make(map[int]*TrieNode),
			depth:    0,
		},
	}
}

// Insert adds an OID-to-name mapping to the trie.
// The OID should be in dotted decimal notation (e.g., ".1.3.6.1.6.3.1.1.5.1").
func (t *OIDTrie) Insert(oid, name string) error {
	if oid == "" || name == "" {
		return nil // Skip empty entries
	}

	components, err := parseOID(oid)
	if err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	current := t.root

	// Traverse/create the path through the trie
	for i, component := range components {
		if current.children == nil {
			current.children = make(map[int]*TrieNode)
		}

		if current.children[component] == nil {
			current.children[component] = &TrieNode{
				children: make(map[int]*TrieNode),
				depth:    i + 1,
			}
		}

		current = current.children[component]
	}

	// Mark as leaf and set name
	current.isLeaf = true
	current.name = name
	t.size++

	return nil
}

// Lookup finds the name associated with an OID.
// Returns empty string if the OID is not found.
func (t *OIDTrie) Lookup(oid string) string {
	if oid == "" {
		return ""
	}

	components, err := parseOID(oid)
	if err != nil {
		return ""
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	current := t.root

	// Traverse the trie following the OID path
	for _, component := range components {
		if current.children == nil {
			return ""
		}

		next, exists := current.children[component]
		if !exists {
			return ""
		}

		current = next
	}

	// Return name if this is a leaf node
	if current.isLeaf {
		return current.name
	}

	return ""
}

// LookupPrefix finds all OIDs that start with the given prefix.
// This is useful for finding all OIDs under a specific branch of the MIB tree.
func (t *OIDTrie) LookupPrefix(prefix string) map[string]string {
	if prefix == "" {
		return make(map[string]string)
	}

	components, err := parseOID(prefix)
	if err != nil {
		return make(map[string]string)
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	current := t.root

	// Navigate to the prefix node
	for _, component := range components {
		if current.children == nil {
			return make(map[string]string)
		}

		next, exists := current.children[component]
		if !exists {
			return make(map[string]string)
		}

		current = next
	}

	// Collect all leaf nodes under this prefix
	result := make(map[string]string)
	t.collectLeaves(current, prefix, result)

	return result
}

// collectLeaves recursively collects all leaf nodes under a given node.
func (t *OIDTrie) collectLeaves(node *TrieNode, currentOID string, result map[string]string) {
	if node.isLeaf {
		result[currentOID] = node.name
	}

	if node.children != nil {
		for component, child := range node.children {
			childOID := currentOID + "." + strconv.Itoa(component)
			t.collectLeaves(child, childOID, result)
		}
	}
}

// Size returns the number of OID entries in the trie.
func (t *OIDTrie) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.size
}

// Clear removes all entries from the trie.
func (t *OIDTrie) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.root = &TrieNode{
		children: make(map[int]*TrieNode),
		depth:    0,
	}
	t.size = 0
}

// GetDepthStats returns statistics about the depth distribution in the trie.
// This is useful for understanding the structure of loaded MIBs and optimizing performance.
func (t *OIDTrie) GetDepthStats() map[int]int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	depthCounts := make(map[int]int)
	t.collectDepthStats(t.root, depthCounts)

	return depthCounts
}

// collectDepthStats recursively collects depth statistics.
func (t *OIDTrie) collectDepthStats(node *TrieNode, depthCounts map[int]int) {
	if node.isLeaf {
		depthCounts[node.depth]++
	}

	if node.children != nil {
		for _, child := range node.children {
			t.collectDepthStats(child, depthCounts)
		}
	}
}

// GetMemoryUsage estimates the memory usage of the trie in bytes.
// This provides an approximation for monitoring and optimization purposes.
func (t *OIDTrie) GetMemoryUsage() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.calculateMemoryUsage(t.root)
}

// calculateMemoryUsage recursively calculates memory usage.
func (t *OIDTrie) calculateMemoryUsage(node *TrieNode) int64 {
	if node == nil {
		return 0
	}

	// Base size of TrieNode struct
	size := int64(64) // Approximate size of struct fields

	// Add size of name string
	size += int64(len(node.name))

	// Add size of children map
	if node.children != nil {
		size += int64(len(node.children) * 16) // Approximate map overhead per entry

		// Recursively calculate children
		for _, child := range node.children {
			size += t.calculateMemoryUsage(child)
		}
	}

	return size
}

// Exists checks if an OID exists in the trie without returning the name.
// This is more efficient than Lookup when you only need to check existence.
func (t *OIDTrie) Exists(oid string) bool {
	if oid == "" {
		return false
	}

	components, err := parseOID(oid)
	if err != nil {
		return false
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	current := t.root

	for _, component := range components {
		if current.children == nil {
			return false
		}

		next, exists := current.children[component]
		if !exists {
			return false
		}

		current = next
	}

	return current.isLeaf
}

// =============================================================================
// Cache Constructor and Methods
// =============================================================================

// NewCache creates a new LRU cache with the specified capacity.
// If capacity is 0 or negative, a default capacity of 1000 is used.
func NewCache(capacity int) *Cache {
	if capacity <= 0 {
		capacity = 1000
	}

	return &Cache{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		lruList:  list.New(),
		stats: CacheStats{
			Capacity: capacity,
		},
	}
}

// Get retrieves a value from the cache and marks it as recently used.
// Returns the value and true if found, empty string and false if not found.
func (c *Cache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, exists := c.items[key]
	if !exists {
		c.stats.Misses++
		c.updateHitRatio()
		return "", false
	}

	// Move to front (most recently used)
	c.lruList.MoveToFront(element)

	// Update entry statistics
	entry, ok := element.Value.(*CacheEntry)
	if !ok {
		c.stats.Misses++
		c.updateHitRatio()
		return "", false
	}
	entry.timestamp = time.Now()
	entry.hitCount++

	// Update cache statistics
	c.stats.Hits++
	c.stats.LastAccess = entry.timestamp
	c.updateHitRatio()

	return entry.value, true
}

// Set adds or updates a key-value pair in the cache.
// If the cache is at capacity, the least recently used item is evicted.
func (c *Cache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if key already exists
	if element, exists := c.items[key]; exists {
		// Update existing entry
		c.lruList.MoveToFront(element)
		entry, ok := element.Value.(*CacheEntry)
		if !ok {
			return
		}
		entry.value = value
		entry.timestamp = time.Now()
		return
	}

	// Create new entry
	entry := &CacheEntry{
		key:       key,
		value:     value,
		timestamp: time.Now(),
		hitCount:  0,
	}

	// Add to front of list
	element := c.lruList.PushFront(entry)
	c.items[key] = element

	// Check if we need to evict
	if c.lruList.Len() > c.capacity {
		c.evictLRU()
	}

	c.stats.Size = len(c.items)
}

// Delete removes a key from the cache.
func (c *Cache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, exists := c.items[key]
	if !exists {
		return false
	}

	c.lruList.Remove(element)
	delete(c.items, key)
	c.stats.Size = len(c.items)

	return true
}

// Clear removes all entries from the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.lruList = list.New()
	c.stats.Size = 0
	c.stats.Hits = 0
	c.stats.Misses = 0
	c.stats.Evictions = 0
	c.stats.HitRatio = 0
}

// Size returns the current number of items in the cache.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Capacity returns the maximum capacity of the cache.
func (c *Cache) Capacity() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.capacity
}

// GetStats returns current cache statistics.
func (c *Cache) GetStats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := c.stats
	stats.Size = len(c.items)
	return stats
}

// GetTopEntries returns the most frequently accessed cache entries.
// This is useful for understanding cache usage patterns.
func (c *Cache) GetTopEntries(limit int) []CacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if limit <= 0 || limit > len(c.items) {
		limit = len(c.items)
	}

	entries := make([]CacheEntry, 0, len(c.items))

	// Collect all entries
	for element := c.lruList.Front(); element != nil; element = element.Next() {
		entry, ok := element.Value.(*CacheEntry)
		if !ok {
			continue
		}
		entries = append(entries, *entry)
	}

	// Sort by hit count (descending)
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].hitCount < entries[j].hitCount {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	if limit < len(entries) {
		entries = entries[:limit]
	}

	return entries
}

// Resize changes the capacity of the cache.
// If the new capacity is smaller than the current size, LRU entries are evicted.
func (c *Cache) Resize(newCapacity int) {
	if newCapacity <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.capacity = newCapacity
	c.stats.Capacity = newCapacity

	// Evict entries if necessary
	for c.lruList.Len() > c.capacity {
		c.evictLRU()
	}
}

// GetMemoryUsage estimates the memory usage of the cache in bytes.
func (c *Cache) GetMemoryUsage() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var totalSize int64

	// Base overhead for maps and list
	totalSize += int64(len(c.items) * 64)    // Approximate overhead per map entry
	totalSize += int64(c.lruList.Len() * 48) // Approximate overhead per list element

	// Calculate string storage
	for element := c.lruList.Front(); element != nil; element = element.Next() {
		entry, ok := element.Value.(*CacheEntry)
		if !ok {
			continue
		}
		totalSize += int64(len(entry.key) + len(entry.value))
	}

	return totalSize
}

// evictLRU removes the least recently used item from the cache.
// Must be called with mutex locked.
func (c *Cache) evictLRU() {
	if c.lruList.Len() == 0 {
		return
	}

	// Get the least recently used item (back of list)
	element := c.lruList.Back()
	if element == nil {
		return
	}

	entry, ok := element.Value.(*CacheEntry)
	if !ok {
		return
	}

	// Remove from both data structures
	c.lruList.Remove(element)
	delete(c.items, entry.key)

	// Update statistics
	c.stats.Evictions++
	c.stats.LastEviction = time.Now()
	c.stats.Size = len(c.items)
}

// updateHitRatio calculates and updates the cache hit ratio.
// Must be called with mutex locked.
func (c *Cache) updateHitRatio() {
	total := c.stats.Hits + c.stats.Misses
	if total > 0 {
		c.stats.HitRatio = float64(c.stats.Hits) / float64(total)
	} else {
		c.stats.HitRatio = 0
	}
}

// Warmup pre-loads the cache with frequently used OID translations.
// This can improve performance by avoiding cache misses for common OIDs.
func (c *Cache) Warmup(translations map[string]string) {
	for oid, name := range translations {
		c.Set(oid, name)
	}
}

// GetCommonOIDTranslations returns a map of common SNMP OID translations
// that can be used for cache warmup.
func GetCommonOIDTranslations() map[string]string {
	return map[string]string{
		".1.3.6.1.6.3.1.1.5.1": "coldStart",
		".1.3.6.1.6.3.1.1.5.2": "warmStart",
		".1.3.6.1.6.3.1.1.5.3": "linkDown",
		".1.3.6.1.6.3.1.1.5.4": "linkUp",
		".1.3.6.1.6.3.1.1.5.5": "authenticationFailure",
		".1.3.6.1.6.3.1.1.5.6": "egpNeighborLoss",
		".1.3.6.1.2.1.1.1.0":   "sysDescr",
		".1.3.6.1.2.1.1.2.0":   "sysObjectID",
		".1.3.6.1.2.1.1.3.0":   "sysUpTime",
		".1.3.6.1.2.1.1.4.0":   "sysContact",
		".1.3.6.1.2.1.1.5.0":   "sysName",
		".1.3.6.1.2.1.1.6.0":   "sysLocation",
		".1.3.6.1.2.1.1.7.0":   "sysServices",
	}
}

// =============================================================================
// MIB Parser Constructor and Methods
// =============================================================================

// NewMIBParser creates a new MIB parser with compiled regular expressions.
func NewMIBParser() *MIBParser {
	return &MIBParser{
		// Match OBJECT-TYPE definitions
		objectTypeRegex: regexp.MustCompile(`(?i)^\s*([a-zA-Z][a-zA-Z0-9-]*)\s+OBJECT-TYPE`),

		// Match NOTIFICATION-TYPE definitions
		notificationRegex: regexp.MustCompile(`(?i)^\s*([a-zA-Z][a-zA-Z0-9-]*)\s+NOTIFICATION-TYPE`),

		// Match OID assignments like: name OBJECT IDENTIFIER ::= { parent child }
		oidAssignmentRegex: regexp.MustCompile(`(?i)^\s*([a-zA-Z][a-zA-Z0-9-]*)\s+OBJECT\s+IDENTIFIER\s*::=\s*\{\s*([^}]+)\s*\}`),

		// Match MODULE-IDENTITY definitions
		moduleIdentityRegex: regexp.MustCompile(`(?i)^\s*([a-zA-Z][a-zA-Z0-9-]*)\s+MODULE-IDENTITY`),

		oidContext: make(map[string]string),
	}
}

// ParseFile parses a single MIB file and returns extracted OID entries.
func (p *MIBParser) ParseFile(filename string) ([]OIDEntry, error) {
	file, err := p.openAndValidateFile(filename)
	if err != nil {
		return nil, err
	}
	defer func() {
		file.Close()
	}()

	// Reset parser state
	p.resetParserState()

	return p.parseFileContent(file)
}

// openAndValidateFile opens and validates a MIB file.
func (p *MIBParser) openAndValidateFile(filename string) (*os.File, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open MIB file: %w", err)
	}

	// First validate the file
	if err := p.ValidateMIBFile(filename); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			return nil, fmt.Errorf("invalid MIB file: %w", err)
		}
		return nil, fmt.Errorf("invalid MIB file: %w", err)
	}

	return file, nil
}

// resetParserState resets the parser state for a new file.
func (p *MIBParser) resetParserState() {
	p.oidContext = make(map[string]string)
	p.currentModule = ""
}

// parseFileContent parses the content of an opened MIB file.
func (p *MIBParser) parseFileContent(file *os.File) ([]OIDEntry, error) {
	var entries []OIDEntry
	scanner := bufio.NewScanner(file)

	// Track multi-line constructs
	constructState := &constructParseState{
		currentConstruct: strings.Builder{},
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}

		if newEntries := p.processLine(line, constructState); newEntries != nil {
			entries = append(entries, newEntries...)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading MIB file: %w", err)
	}

	return entries, nil
}

// constructParseState holds state for parsing multi-line constructs.
type constructParseState struct {
	currentConstruct strings.Builder
	inConstruct      bool
	constructName    string
	constructType    string
}

// processLine processes a single line and returns any extracted entries.
func (p *MIBParser) processLine(line string, state *constructParseState) []OIDEntry {
	// Handle multi-line constructs
	if state.inConstruct {
		return p.handleMultiLineConstruct(line, state)
	}

	// Check for start of new constructs
	return p.handleNewConstruct(line, state)
}

// handleMultiLineConstruct handles lines within a multi-line construct.
func (p *MIBParser) handleMultiLineConstruct(line string, state *constructParseState) []OIDEntry {
	state.currentConstruct.WriteString(" ")
	state.currentConstruct.WriteString(line)

	// Check if construct is complete
	if strings.Contains(line, "}") || strings.Contains(line, "::=") {
		fullConstruct := state.currentConstruct.String()

		var entries []OIDEntry
		if entry := p.parseConstruct(state.constructName, state.constructType, fullConstruct); entry != nil {
			entries = append(entries, *entry)
		}

		// Reset state
		state.inConstruct = false
		state.currentConstruct.Reset()
		state.constructName = ""
		state.constructType = ""

		return entries
	}

	return nil
}

// handleNewConstruct handles the start of new constructs and simple assignments.
func (p *MIBParser) handleNewConstruct(line string, state *constructParseState) []OIDEntry {
	// Check for start of new constructs
	if matches := p.objectTypeRegex.FindStringSubmatch(line); matches != nil {
		state.constructName = matches[1]
		state.constructType = "OBJECT-TYPE"
		state.currentConstruct.WriteString(line)
		state.inConstruct = true
		return nil
	}

	if matches := p.notificationRegex.FindStringSubmatch(line); matches != nil {
		state.constructName = matches[1]
		state.constructType = "NOTIFICATION-TYPE"
		state.currentConstruct.WriteString(line)
		state.inConstruct = true
		return nil
	}

	if matches := p.moduleIdentityRegex.FindStringSubmatch(line); matches != nil {
		p.currentModule = matches[1]
		state.constructName = matches[1]
		state.constructType = "MODULE-IDENTITY"
		state.currentConstruct.WriteString(line)
		state.inConstruct = true
		return nil
	}

	// Handle simple OID assignments on single lines
	return p.handleSimpleOIDAssignment(line)
}

// handleSimpleOIDAssignment handles simple OID assignments on single lines.
func (p *MIBParser) handleSimpleOIDAssignment(line string) []OIDEntry {
	if matches := p.oidAssignmentRegex.FindStringSubmatch(line); matches != nil {
		name := matches[1]
		oidPart := matches[2]

		if oid := p.parseOIDAssignment(oidPart); oid != "" {
			entry := OIDEntry{
				OID:    oid,
				Name:   name,
				Type:   "OBJECT IDENTIFIER",
				Module: p.currentModule,
			}

			// Store in context for future references
			p.oidContext[name] = oid

			return []OIDEntry{entry}
		}
	}

	return nil
}

// parseConstruct parses a complete MIB construct and extracts OID information.
func (p *MIBParser) parseConstruct(name, constructType, content string) *OIDEntry {
	// Look for OID assignment within the construct
	oidRegex := regexp.MustCompile(`::=\s*\{\s*([^}]+)\s*\}`)
	matches := oidRegex.FindStringSubmatch(content)

	if matches == nil {
		return nil
	}

	oidPart := matches[1]
	oid := p.parseOIDAssignment(oidPart)

	if oid == "" {
		return nil
	}

	entry := &OIDEntry{
		OID:    oid,
		Name:   name,
		Type:   constructType,
		Module: p.currentModule,
	}

	// Extract description if present
	if desc := p.extractDescription(content); desc != "" {
		entry.Description = desc
	}

	// Store in context for future references
	p.oidContext[name] = oid

	return entry
}

// parseOIDAssignment parses an OID assignment like "{ iso org(3) dod(6) internet(1) mgmt(2) mib-2(1) system(1) sysDescr(1) }"
func (p *MIBParser) parseOIDAssignment(oidPart string) string {
	// Clean up the OID part
	oidPart = strings.TrimSpace(oidPart)

	// Handle different OID formats
	var components []string

	// Split by whitespace and parse each component
	parts := strings.Fields(oidPart)

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if component := p.parseOIDComponent(part); component != "" {
			components = append(components, component)
		}
	}

	if len(components) == 0 {
		return ""
	}

	// Join components to form complete OID
	return "." + strings.Join(components, ".")
}

// parseOIDComponent parses a single OID component and returns its numeric value.
func (p *MIBParser) parseOIDComponent(part string) string {
	// Handle formats like "iso", "org(3)", "3", etc.
	if strings.Contains(part, "(") && strings.Contains(part, ")") {
		// Extract number from parentheses
		numRegex := regexp.MustCompile(`\((\d+)\)`)
		if numMatches := numRegex.FindStringSubmatch(part); numMatches != nil {
			return numMatches[1]
		}
		return ""
	}

	if num, err := strconv.Atoi(part); err == nil {
		// Direct numeric value
		return strconv.Itoa(num)
	}

	// Try to resolve symbolic name
	if resolvedOID, exists := p.oidContext[part]; exists {
		// If it's a complete OID, use it as base
		if strings.HasPrefix(resolvedOID, ".") {
			return resolvedOID
		}
		return resolvedOID
	}

	// Handle well-known symbolic names
	return p.resolveWellKnownName(part)
}

// resolveWellKnownName resolves common symbolic names to their numeric OID components.
func (p *MIBParser) resolveWellKnownName(name string) string {
	wellKnownNames := map[string]string{
		"iso":          "1",
		"org":          "3",
		"dod":          "6",
		"internet":     "1",
		"directory":    "1",
		"mgmt":         "2",
		"mib-2":        "1",
		"experimental": "3",
		"private":      "4",
		"enterprises":  "1",
		"security":     "5",
		"snmpV2":       "6",
		"system":       "1",
		"interfaces":   "2",
		"at":           "3",
		"ip":           "4",
		"icmp":         "5",
		"tcp":          "6",
		"udp":          "7",
		"egp":          "8",
		"transmission": "10",
		"snmp":         "11",
	}

	if oid, exists := wellKnownNames[strings.ToLower(name)]; exists {
		return oid
	}

	return ""
}

// extractDescription extracts the DESCRIPTION field from a MIB construct.
func (p *MIBParser) extractDescription(content string) string {
	descRegex := regexp.MustCompile(`(?i)DESCRIPTION\s*"([^"]*)"`)
	matches := descRegex.FindStringSubmatch(content)

	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	return ""
}

// ParseDirectory parses all MIB files in a directory.
func (p *MIBParser) ParseDirectory(dirPath string) ([]OIDEntry, error) {
	var allEntries []OIDEntry

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		if !isMIBFile(filename) {
			continue
		}

		fullPath := dirPath + "/" + filename
		fileEntries, err := p.ParseFile(fullPath)
		if err != nil {
			// Log error but continue with other files
			fmt.Printf("Warning: failed to parse MIB file %s: %v\n", fullPath, err)
			continue
		}

		allEntries = append(allEntries, fileEntries...)
	}

	return allEntries, nil
}

// ValidateMIBFile performs basic validation on a MIB file.
func (p *MIBParser) ValidateMIBFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer func() {
		file.Close()
	}()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	hasModuleIdentity := false
	hasDefinitions := false

	for scanner.Scan() {
		lineCount++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}

		if p.moduleIdentityRegex.MatchString(line) {
			hasModuleIdentity = true
		}

		if p.objectTypeRegex.MatchString(line) || p.notificationRegex.MatchString(line) {
			hasDefinitions = true
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	if lineCount == 0 {
		return errors.New("file is empty")
	}

	if !hasModuleIdentity && !hasDefinitions {
		return errors.New("file does not appear to be a valid MIB file")
	}

	return nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// parseOID converts an OID string to a slice of integer components.
// Handles both formats: "1.3.6.1" and ".1.3.6.1"
func parseOID(oid string) ([]int, error) {
	// Remove leading dot if present
	oid = strings.TrimPrefix(oid, ".")

	if oid == "" {
		return []int{}, nil
	}

	parts := strings.Split(oid, ".")
	components := make([]int, len(parts))

	for i, part := range parts {
		if part == "" {
			continue // Skip empty parts from double dots
		}

		num, err := strconv.Atoi(part)
		if err != nil {
			return nil, err
		}

		if num < 0 {
			return nil, strconv.ErrRange
		}

		components[i] = num
	}

	return components, nil
}

// Init initializes the translator with MIB files from the specified directory.
func (t *translator) Init(mibDir string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.initialized {
		return errors.New("translator already initialized")
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
		return "", errors.New("translator not initialized")
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
		return nil, errors.New("translator not initialized")
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
		if err := t.trie.Insert(entry.OID, entry.Name); err != nil {
			// Log error but continue with other entries
			continue
		}
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
		if err := t.trie.Insert(entry.OID, entry.Name); err != nil {
			// Log error but continue with other entries
			continue
		}
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
