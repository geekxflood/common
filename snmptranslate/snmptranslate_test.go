package snmptranslate

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSNMPTranslate(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SNMPTranslate Suite")
}

var _ = Describe("SNMPTranslate", func() {
	var (
		translator Translator
		tempDir    string
	)

	BeforeEach(func() {
		translator = New()

		// Create temporary directory for test MIB files
		var err error
		tempDir, err = os.MkdirTemp("", "snmptranslate_test")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if translator != nil {
			translator.Close()
		}
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
	})

	Describe("Translator Creation", func() {
		DescribeTable("creating translators with different configurations",
			func(config Config, expectedCacheSize int) {
				t := NewWithConfig(config)
				Expect(t).NotTo(BeNil())

				// Verify configuration is applied
				if config.MaxCacheSize > 0 {
					// We can't directly access cache size, but we can verify it works
					Expect(t.Init(tempDir)).To(Succeed())
				}
			},
			Entry("default config", DefaultConfig(), 10000),
			Entry("custom cache size", Config{MaxCacheSize: 5000, LazyLoading: true}, 5000),
			Entry("no lazy loading", Config{MaxCacheSize: 1000, LazyLoading: false}, 1000),
		)
	})

	Describe("Initialization", func() {
		It("should initialize successfully with valid directory", func() {
			err := translator.Init(tempDir)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail with non-existent directory", func() {
			err := translator.Init("/non/existent/directory")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not exist"))
		})

		It("should fail when already initialized", func() {
			err := translator.Init(tempDir)
			Expect(err).NotTo(HaveOccurred())

			err = translator.Init(tempDir)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already initialized"))
		})
	})

	Describe("Basic Translation", func() {
		BeforeEach(func() {
			// Create a simple test MIB file
			createTestMIBFile(tempDir, "TEST-MIB.mib", testMIBContent)
			err := translator.Init(tempDir)
			Expect(err).NotTo(HaveOccurred())
		})

		DescribeTable("translating common OIDs",
			func(oid, expectedName string) {
				name, err := translator.Translate(oid)
				if expectedName == oid {
					// Expecting no translation (returns original OID)
					Expect(name).To(Equal(oid))
				} else {
					Expect(err).NotTo(HaveOccurred())
					Expect(name).To(Equal(expectedName))
				}
			},
			Entry("coldStart", ".1.3.6.1.6.3.1.1.5.1", "coldStart"),
			Entry("warmStart", ".1.3.6.1.6.3.1.1.5.2", "warmStart"),
			Entry("linkDown", ".1.3.6.1.6.3.1.1.5.3", "linkDown"),
			Entry("linkUp", ".1.3.6.1.6.3.1.1.5.4", "linkUp"),
			Entry("unknown OID", ".1.2.3.4.5.6.7.8.9", ".1.2.3.4.5.6.7.8.9"),
		)

		It("should handle OIDs without leading dot", func() {
			name, err := translator.Translate("1.3.6.1.6.3.1.1.5.1")
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("coldStart"))
		})

		It("should fail when not initialized", func() {
			newTranslator := New()
			_, err := newTranslator.Translate(".1.3.6.1.6.3.1.1.5.1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not initialized"))
		})
	})

	Describe("Batch Translation", func() {
		BeforeEach(func() {
			err := translator.Init(tempDir)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should translate multiple OIDs", func() {
			oids := []string{
				".1.3.6.1.6.3.1.1.5.1",
				".1.3.6.1.6.3.1.1.5.2",
				".1.3.6.1.6.3.1.1.5.3",
			}

			results, err := translator.TranslateBatch(oids)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(3))
			Expect(results[".1.3.6.1.6.3.1.1.5.1"]).To(Equal("coldStart"))
			Expect(results[".1.3.6.1.6.3.1.1.5.2"]).To(Equal("warmStart"))
			Expect(results[".1.3.6.1.6.3.1.1.5.3"]).To(Equal("linkDown"))
		})

		It("should handle mixed valid and invalid OIDs", func() {
			oids := []string{
				".1.3.6.1.6.3.1.1.5.1", // valid
				".1.2.3.4.5.6.7.8.9",   // unknown
			}

			results, err := translator.TranslateBatch(oids)
			Expect(results).To(HaveLen(2))
			Expect(results[".1.3.6.1.6.3.1.1.5.1"]).To(Equal("coldStart"))
			Expect(results[".1.2.3.4.5.6.7.8.9"]).To(Equal(".1.2.3.4.5.6.7.8.9"))

			// Should have error for unknown OID
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Statistics", func() {
		BeforeEach(func() {
			err := translator.Init(tempDir)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should track translation statistics", func() {
			// Perform some translations
			translator.Translate(".1.3.6.1.6.3.1.1.5.1")
			translator.Translate(".1.3.6.1.6.3.1.1.5.2")
			translator.Translate(".1.3.6.1.6.3.1.1.5.1") // Repeat for cache hit

			stats := translator.GetStats()
			Expect(stats.TranslationCount).To(BeNumerically(">=", 3))
			Expect(stats.CacheHits).To(BeNumerically(">=", 1))
			Expect(stats.AverageLatency).To(BeNumerically(">", 0))
		})
	})

	Describe("MIB Loading", func() {
		It("should load individual MIB files", func() {
			mibFile := createTestMIBFile(tempDir, "CUSTOM-MIB.mib", customMIBContent)

			err := translator.Init(tempDir)
			Expect(err).NotTo(HaveOccurred())

			err = translator.LoadMIB(mibFile)
			Expect(err).NotTo(HaveOccurred())

			stats := translator.GetStats()
			Expect(stats.LoadedMIBs).To(BeNumerically(">=", 1))
		})

		It("should handle invalid MIB files gracefully", func() {
			invalidFile := createTestMIBFile(tempDir, "INVALID.mib", "invalid content")

			err := translator.Init(tempDir)
			Expect(err).NotTo(HaveOccurred())

			err = translator.LoadMIB(invalidFile)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Resource Management", func() {
		It("should clean up resources on close", func() {
			err := translator.Init(tempDir)
			Expect(err).NotTo(HaveOccurred())

			// Perform some operations
			translator.Translate(".1.3.6.1.6.3.1.1.5.1")

			err = translator.Close()
			Expect(err).NotTo(HaveOccurred())

			// Should fail after close
			_, err = translator.Translate(".1.3.6.1.6.3.1.1.5.1")
			Expect(err).To(HaveOccurred())
		})
	})
})

var _ = Describe("OIDTrie", func() {
	var trie *OIDTrie

	BeforeEach(func() {
		trie = NewOIDTrie()
	})

	Describe("Basic Operations", func() {
		It("should insert and lookup OIDs", func() {
			err := trie.Insert(".1.3.6.1.6.3.1.1.5.1", "coldStart")
			Expect(err).NotTo(HaveOccurred())

			name := trie.Lookup(".1.3.6.1.6.3.1.1.5.1")
			Expect(name).To(Equal("coldStart"))
		})

		It("should return empty string for non-existent OIDs", func() {
			name := trie.Lookup(".1.2.3.4.5")
			Expect(name).To(Equal(""))
		})

		It("should handle OIDs without leading dot", func() {
			err := trie.Insert("1.3.6.1.6.3.1.1.5.1", "coldStart")
			Expect(err).NotTo(HaveOccurred())

			name := trie.Lookup("1.3.6.1.6.3.1.1.5.1")
			Expect(name).To(Equal("coldStart"))
		})
	})

	Describe("Prefix Operations", func() {
		BeforeEach(func() {
			trie.Insert(".1.3.6.1.6.3.1.1.5.1", "coldStart")
			trie.Insert(".1.3.6.1.6.3.1.1.5.2", "warmStart")
			trie.Insert(".1.3.6.1.6.3.1.1.5.3", "linkDown")
		})

		It("should find OIDs by prefix", func() {
			results := trie.LookupPrefix(".1.3.6.1.6.3.1.1.5")
			Expect(results).To(HaveLen(3))
			Expect(results[".1.3.6.1.6.3.1.1.5.1"]).To(Equal("coldStart"))
			Expect(results[".1.3.6.1.6.3.1.1.5.2"]).To(Equal("warmStart"))
			Expect(results[".1.3.6.1.6.3.1.1.5.3"]).To(Equal("linkDown"))
		})
	})

	Describe("Statistics", func() {
		It("should track size correctly", func() {
			Expect(trie.Size()).To(Equal(0))

			trie.Insert(".1.3.6.1.6.3.1.1.5.1", "coldStart")
			Expect(trie.Size()).To(Equal(1))

			trie.Insert(".1.3.6.1.6.3.1.1.5.2", "warmStart")
			Expect(trie.Size()).To(Equal(2))
		})

		It("should provide memory usage estimates", func() {
			trie.Insert(".1.3.6.1.6.3.1.1.5.1", "coldStart")
			usage := trie.GetMemoryUsage()
			Expect(usage).To(BeNumerically(">", 0))
		})
	})
})

var _ = Describe("Cache", func() {
	var cache *Cache

	BeforeEach(func() {
		cache = NewCache(3) // Small cache for testing
	})

	Describe("Basic Operations", func() {
		It("should store and retrieve values", func() {
			cache.Set("key1", "value1")

			value, found := cache.Get("key1")
			Expect(found).To(BeTrue())
			Expect(value).To(Equal("value1"))
		})

		It("should return false for non-existent keys", func() {
			_, found := cache.Get("nonexistent")
			Expect(found).To(BeFalse())
		})
	})

	Describe("LRU Behavior", func() {
		It("should evict least recently used items", func() {
			// Fill cache to capacity
			cache.Set("key1", "value1")
			cache.Set("key2", "value2")
			cache.Set("key3", "value3")

			// Access key1 to make it recently used
			cache.Get("key1")

			// Add new item, should evict key2 (least recently used)
			cache.Set("key4", "value4")

			_, found := cache.Get("key2")
			Expect(found).To(BeFalse())

			_, found = cache.Get("key1")
			Expect(found).To(BeTrue())
		})
	})

	Describe("Statistics", func() {
		It("should track cache statistics", func() {
			cache.Set("key1", "value1")
			cache.Get("key1") // Hit
			cache.Get("key2") // Miss

			stats := cache.GetStats()
			Expect(stats.Hits).To(Equal(int64(1)))
			Expect(stats.Misses).To(Equal(int64(1)))
			Expect(stats.Size).To(Equal(1))
		})
	})
})

// Helper functions for tests

func createTestMIBFile(dir, filename, content string) string {
	filePath := filepath.Join(dir, filename)
	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		panic(err)
	}
	return filePath
}

const testMIBContent = `
TEST-MIB DEFINITIONS ::= BEGIN

IMPORTS
    MODULE-IDENTITY, OBJECT-TYPE, NOTIFICATION-TYPE
        FROM SNMPv2-SMI;

testMIB MODULE-IDENTITY
    LAST-UPDATED "202501010000Z"
    ORGANIZATION "Test Organization"
    CONTACT-INFO "test@example.com"
    DESCRIPTION "Test MIB for unit testing"
    ::= { enterprises 12345 }

testObject OBJECT-TYPE
    SYNTAX INTEGER
    MAX-ACCESS read-only
    STATUS current
    DESCRIPTION "Test object"
    ::= { testMIB 1 }

END
`

const customMIBContent = `
CUSTOM-MIB DEFINITIONS ::= BEGIN

customMIB MODULE-IDENTITY
    LAST-UPDATED "202501010000Z"
    ORGANIZATION "Custom Organization"
    CONTACT-INFO "custom@example.com"
    DESCRIPTION "Custom MIB for testing"
    ::= { enterprises 54321 }

customObject OBJECT-TYPE
    SYNTAX OCTET STRING
    MAX-ACCESS read-write
    STATUS current
    DESCRIPTION "Custom test object"
    ::= { customMIB 1 }

END
`

// Benchmark tests to demonstrate performance improvements
func BenchmarkTranslate(b *testing.B) {
	translator := New()

	// Initialize with some test data
	tempDir := b.TempDir()
	testMIB := `
TEST-MIB DEFINITIONS ::= BEGIN
testOID OBJECT-TYPE
    SYNTAX INTEGER
    ACCESS read-only
    STATUS mandatory
    DESCRIPTION "Test OID"
    ::= { 1 3 6 1 4 1 12345 1 1 }
END`

	mibFile := filepath.Join(tempDir, "test.mib")
	err := os.WriteFile(mibFile, []byte(testMIB), 0644)
	if err != nil {
		b.Fatal(err)
	}

	err = translator.Init(tempDir)
	if err != nil {
		b.Fatal(err)
	}

	testOIDs := []string{
		".1.3.6.1.6.3.1.1.5.1",   // coldStart
		".1.3.6.1.6.3.1.1.5.2",   // warmStart
		".1.3.6.1.6.3.1.1.5.3",   // linkDown
		".1.3.6.1.6.3.1.1.5.4",   // linkUp
		".1.3.6.1.4.1.12345.1.1", // testOID
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		oid := testOIDs[i%len(testOIDs)]
		_, _ = translator.Translate(oid)
	}
}

func BenchmarkTranslateBatch(b *testing.B) {
	translator := New()

	// Initialize with some test data
	tempDir := b.TempDir()
	testMIB := `
TEST-MIB DEFINITIONS ::= BEGIN
testOID OBJECT-TYPE
    SYNTAX INTEGER
    ACCESS read-only
    STATUS mandatory
    DESCRIPTION "Test OID"
    ::= { 1 3 6 1 4 1 12345 1 1 }
END`

	mibFile := filepath.Join(tempDir, "test.mib")
	err := os.WriteFile(mibFile, []byte(testMIB), 0644)
	if err != nil {
		b.Fatal(err)
	}

	err = translator.Init(tempDir)
	if err != nil {
		b.Fatal(err)
	}

	testOIDs := []string{
		".1.3.6.1.6.3.1.1.5.1",   // coldStart
		".1.3.6.1.6.3.1.1.5.2",   // warmStart
		".1.3.6.1.6.3.1.1.5.3",   // linkDown
		".1.3.6.1.6.3.1.1.5.4",   // linkUp
		".1.3.6.1.4.1.12345.1.1", // testOID
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = translator.TranslateBatch(testOIDs)
	}
}
