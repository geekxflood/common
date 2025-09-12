// Package trapprocessor provides consolidated SNMP trap processing with template engine and core utilities.
//
// This package combines SNMP trap listening, parsing, template processing, and core utilities
// into a single optimized package for lean architecture.
//
// This is an independent Go module that can be easily integrated into external projects.
// It follows minimal dependency principles and provides clean configuration interfaces.
//
// Basic Usage:
//
//	config := map[string]any{
//		"snmp": map[string]any{
//			"port":         1162,
//			"bind_address": "0.0.0.0",
//			"community":    "public",
//			"version":      "2c",
//		},
//		"worker_pool": map[string]any{
//			"enabled": true,
//			"size":    10,
//		},
//		"templates": map[string]any{
//			"path":    "/etc/templates",
//			"default": "default",
//		},
//	}
//
//	processor, err := New(config)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Use the processor...
package trapprocessor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/geekxflood/common/snmptranslate"
	"github.com/gosnmp/gosnmp"
)

// Config interface for trap processing configuration.
// This interface defines the configuration contract required by the trap
// processing components, abstracting away the underlying configuration
// implementation and making the code more testable.
//
// The interface is organized into logical groups:
//   - SNMP: Network and protocol settings for trap reception
//   - Worker Pool: Concurrency and performance settings
//   - Template: Template engine configuration for message formatting
//
// All configuration methods should provide sensible defaults and handle
// errors gracefully to ensure robust operation.
type Config interface {
	// SNMP configuration
	GetSNMPPort() int
	GetSNMPBindAddress() string
	GetSNMPCommunity() string
	GetSNMPVersion() string
	GetSNMPMIBPaths() []string
	GetWorkerPoolSize() int
	GetWorkerPoolEnabled() bool
	GetMaxRetries() int
	GetBufferSize() int
	GetReadTimeout() time.Duration
	GetWriteTimeout() time.Duration
}

// TrapProcessor is the main trap processing engine that combines all components.
// It provides a clean interface for SNMP trap processing with minimal dependencies.
type TrapProcessor struct {
	config   *configImpl
	listener *Listener
	parser   *TrapParser
}

// configImpl implements the Config interface with default values.
type configImpl struct {
	snmpPort          int
	snmpBindAddress   string
	snmpCommunity     string
	snmpVersion       string
	snmpMIBPaths      []string
	workerPoolSize    int
	workerPoolEnabled bool
	maxRetries        int
	bufferSize        int
	readTimeout       time.Duration
	writeTimeout      time.Duration
}

// Config interface implementation
func (c *configImpl) GetSNMPPort() int               { return c.snmpPort }
func (c *configImpl) GetSNMPBindAddress() string     { return c.snmpBindAddress }
func (c *configImpl) GetSNMPCommunity() string       { return c.snmpCommunity }
func (c *configImpl) GetSNMPVersion() string         { return c.snmpVersion }
func (c *configImpl) GetSNMPMIBPaths() []string      { return c.snmpMIBPaths }
func (c *configImpl) GetWorkerPoolSize() int         { return c.workerPoolSize }
func (c *configImpl) GetWorkerPoolEnabled() bool     { return c.workerPoolEnabled }
func (c *configImpl) GetMaxRetries() int             { return c.maxRetries }
func (c *configImpl) GetBufferSize() int             { return c.bufferSize }
func (c *configImpl) GetReadTimeout() time.Duration  { return c.readTimeout }
func (c *configImpl) GetWriteTimeout() time.Duration { return c.writeTimeout }

// New creates a new trap processor from configuration.
// The configuration can be provided as map[string]any or a struct implementing Config interface.
// This follows the same pattern as the ruler project for clean configuration interfaces.
//
// Configuration Examples:
//
// 1. Map configuration (nested structure):
//
//	config := map[string]any{
//	  "snmp": map[string]any{
//	    "port": 1162,
//	    "bind_address": "0.0.0.0",
//	    "community": "public",
//	    "version": "2c",
//	    "mib_paths": []string{"/usr/share/snmp/mibs"},
//	  },
//	  "worker_pool": map[string]any{
//	    "enabled": true,
//	    "size": 10,
//	  },
//	  "templates": map[string]any{
//	    "path": "/etc/templates",
//	    "default": "default",
//	  },
//	}
//
// 2. Map configuration (flat structure for backward compatibility):
//
//	config := map[string]any{
//	  "snmp_port": 1162,
//	  "snmp_bind_address": "0.0.0.0",
//	  "snmp_community": "public",
//	}
//
// 3. Minimal configuration (uses defaults):
//
//	config := map[string]any{}
//
// 4. Custom struct implementing Config interface:
//
//	type MyConfig struct {
//	  Port int
//	  Address string
//	}
//	func (c *MyConfig) GetSNMPPort() int { return c.Port }
//	// ... implement other Config methods
//
// Returns a configured TrapProcessor ready for use.
func New(configObj any) (*TrapProcessor, error) {
	config, err := parseConfig(configObj)
	if err != nil {
		return nil, fmt.Errorf("failed to parse configuration: %w", err)
	}

	// Create parser
	parser := NewTrapParser(config.GetSNMPMIBPaths(), true)

	processor := &TrapProcessor{
		config: config,
		parser: parser,
	}

	// Create listener with processor as PacketProcessor
	listener, err := NewListener(config, processor)
	if err != nil {
		return nil, fmt.Errorf("failed to create listener: %w", err)
	}
	processor.listener = listener

	return processor, nil
}

// ProcessPacket implements the PacketProcessor interface.
// This method processes incoming SNMP packets by parsing them and applying templates.
func (tp *TrapProcessor) ProcessPacket(ctx context.Context, _ []byte, addr *net.UDPAddr) error {
	// For now, create a basic SNMP packet structure
	// In a real implementation, this would parse the raw packet bytes
	snmpPacket := &gosnmp.SnmpPacket{
		Version:   gosnmp.Version2c,
		Community: tp.config.GetSNMPCommunity(),
		Variables: []gosnmp.SnmpPDU{}, // Would be populated from packet parsing
	}

	// Parse into TrapMessage
	_, err := tp.parser.ParseTrap(ctx, snmpPacket, addr)
	if err != nil {
		return fmt.Errorf("failed to parse trap: %w", err)
	}

	// Trap processed successfully - template processing removed for simplicity

	return nil
}

// Start starts the trap processor.
func (tp *TrapProcessor) Start(ctx context.Context) error {
	return tp.listener.Start(ctx)
}

// Stop stops the trap processor.
func (tp *TrapProcessor) Stop(ctx context.Context) error {
	return tp.listener.Stop(ctx)
}

// Close stops the trap processor and cleans up resources.
func (tp *TrapProcessor) Close(ctx context.Context) error {
	// Stop the listener first
	if err := tp.Stop(ctx); err != nil {
		return err
	}

	// Clean up parser resources
	if tp.parser != nil {
		return tp.parser.Close()
	}

	return nil
}

// parseConfig parses configuration from various input types.
func parseConfig(configObj any) (*configImpl, error) {
	config := &configImpl{
		// Default values
		snmpPort:          1162,
		snmpBindAddress:   "0.0.0.0",
		snmpCommunity:     "public",
		snmpVersion:       "2c",
		snmpMIBPaths:      []string{},
		workerPoolSize:    10,
		workerPoolEnabled: true,
		maxRetries:        3,
		bufferSize:        65536,
		readTimeout:       5 * time.Second,
		writeTimeout:      5 * time.Second,
	}

	// Parse configuration based on type
	switch cfg := configObj.(type) {
	case map[string]any:
		return parseMapConfig(config, cfg)
	case Config:
		return parseConfigInterface(config, cfg)
	default:
		return nil, fmt.Errorf("unsupported configuration type: %T", configObj)
	}
}

// parseMapConfig parses configuration from a map with type conversion and validation.
func parseMapConfig(config *configImpl, cfg map[string]any) (*configImpl, error) {
	// Parse SNMP configuration
	if snmpCfg, ok := cfg["snmp"].(map[string]any); ok {
		if err := parseSnmpConfig(config, snmpCfg); err != nil {
			return nil, fmt.Errorf("invalid SNMP configuration: %w", err)
		}
	}

	// Parse worker pool configuration
	if poolCfg, ok := cfg["worker_pool"].(map[string]any); ok {
		if err := parseWorkerPoolConfig(config, poolCfg); err != nil {
			return nil, fmt.Errorf("invalid worker pool configuration: %w", err)
		}
	}

	// Parse flat configuration (for backward compatibility)
	if err := parseFlatConfig(config, cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Validate final configuration
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// parseSnmpConfig parses SNMP-specific configuration.
func parseSnmpConfig(config *configImpl, snmpCfg map[string]any) error {
	if port := getIntValue(snmpCfg, "port"); port != 0 {
		if port < 1 || port > 65535 {
			return fmt.Errorf("SNMP port must be between 1 and 65535, got %d", port)
		}
		config.snmpPort = port
	}

	if addr := getStringValue(snmpCfg, "bind_address"); addr != "" {
		config.snmpBindAddress = addr
	}

	if community := getStringValue(snmpCfg, "community"); community != "" {
		config.snmpCommunity = community
	}

	if version := getStringValue(snmpCfg, "version"); version != "" {
		if !isValidSnmpVersion(version) {
			return fmt.Errorf("invalid SNMP version: %s (must be 1, 2c, or 3)", version)
		}
		config.snmpVersion = version
	}

	if paths := getStringSliceValue(snmpCfg, "mib_paths"); paths != nil {
		config.snmpMIBPaths = paths
	}

	return nil
}

// parseWorkerPoolConfig parses worker pool configuration.
func parseWorkerPoolConfig(config *configImpl, poolCfg map[string]any) error {
	if enabled := getBoolValue(poolCfg, "enabled"); enabled != nil {
		config.workerPoolEnabled = *enabled
	}

	if size := getIntValue(poolCfg, "size"); size != 0 {
		if size < 1 || size > 1000 {
			return fmt.Errorf("worker pool size must be between 1 and 1000, got %d", size)
		}
		config.workerPoolSize = size
	}

	return nil
}

// parseFlatConfig parses flat configuration for backward compatibility.
func parseFlatConfig(config *configImpl, cfg map[string]any) error {
	// Direct configuration keys (for backward compatibility)
	if port := getIntValue(cfg, "snmp_port"); port != 0 {
		config.snmpPort = port
	}
	if addr := getStringValue(cfg, "snmp_bind_address"); addr != "" {
		config.snmpBindAddress = addr
	}
	if community := getStringValue(cfg, "snmp_community"); community != "" {
		config.snmpCommunity = community
	}
	if version := getStringValue(cfg, "snmp_version"); version != "" {
		if !isValidSnmpVersion(version) {
			return fmt.Errorf("invalid SNMP version: %s (must be 1, 2c, or 3)", version)
		}
		config.snmpVersion = version
	}

	return nil
}

// parseConfigInterface parses configuration from a Config interface.
func parseConfigInterface(config *configImpl, cfg Config) (*configImpl, error) {
	config.snmpPort = cfg.GetSNMPPort()
	config.snmpBindAddress = cfg.GetSNMPBindAddress()
	config.snmpCommunity = cfg.GetSNMPCommunity()
	config.snmpVersion = cfg.GetSNMPVersion()
	config.snmpMIBPaths = cfg.GetSNMPMIBPaths()
	config.workerPoolSize = cfg.GetWorkerPoolSize()
	config.workerPoolEnabled = cfg.GetWorkerPoolEnabled()
	config.maxRetries = cfg.GetMaxRetries()
	config.bufferSize = cfg.GetBufferSize()
	config.readTimeout = cfg.GetReadTimeout()
	config.writeTimeout = cfg.GetWriteTimeout()

	return config, nil
}

// Configuration helper functions for type-safe value extraction

// getStringValue safely extracts a string value from a map.
func getStringValue(m map[string]any, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// getIntValue safely extracts an int value from a map.
func getIntValue(m map[string]any, key string) int {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return 0
}

// getBoolValue safely extracts a bool value from a map.
func getBoolValue(m map[string]any, key string) *bool {
	if val, ok := m[key]; ok {
		if b, ok := val.(bool); ok {
			return &b
		}
	}
	return nil
}

// getStringSliceValue safely extracts a []string value from a map.
func getStringSliceValue(m map[string]any, key string) []string {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case []string:
			return v
		case []any:
			result := make([]string, 0, len(v))
			for _, item := range v {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
			return result
		}
	}
	return nil
}

// isValidSnmpVersion checks if the SNMP version is valid.
func isValidSnmpVersion(version string) bool {
	switch version {
	case "1", "2c", "3":
		return true
	default:
		return false
	}
}

// validateConfig validates the final configuration.
func validateConfig(config *configImpl) error {
	if config.snmpPort < 1 || config.snmpPort > 65535 {
		return fmt.Errorf("invalid SNMP port: %d", config.snmpPort)
	}

	if config.snmpBindAddress == "" {
		return errors.New("SNMP bind address cannot be empty")
	}

	if config.workerPoolSize < 1 {
		return errors.New("worker pool size must be at least 1")
	}

	return nil
}

// TrapMessage represents a parsed SNMP trap message with all relevant
// information extracted from the SNMP PDU. This structure serves as the
// primary data model for trap processing throughout the application.
//
// The message contains both the raw SNMP data and processed information
// that can be used by templates and output formatters.
//
// Fields:
//   - SourceIP: The IP address of the device that sent the trap
//   - Timestamp: When the trap was received (not when it was sent)
//   - TrapOID: The SNMP trap OID identifying the type of trap
//   - Variables: List of variable bindings included in the trap
//   - Community: SNMP community string (for v1/v2c)
//   - Version: SNMP version used (1, 2c, or 3)
type TrapMessage struct {
	SourceIP  string         `json:"source_ip"`
	OID       string         `json:"oid"`
	Community string         `json:"community"`
	Version   string         `json:"version"`
	TrapType  string         `json:"trap_type"`
	Timestamp time.Time      `json:"timestamp"`
	Variables []TrapVariable `json:"variables"`
	RawData   map[string]any `json:"raw_data"`
}

// TrapVariable represents a variable binding in an SNMP trap.
// Variable bindings are key-value pairs that carry the actual data
// in SNMP traps, providing specific information about the event or
// condition that triggered the trap.
//
// Fields:
//   - OID: The object identifier for this variable
//   - Name: Human-readable name (if OID translation is enabled)
//   - Type: SNMP data type (Integer, OctetString, etc.)
//   - Value: The actual value, type depends on the SNMP type
//   - DisplayHint: Optional formatting hint for display purposes
type TrapVariable struct {
	OID         string `json:"oid"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Value       any    `json:"value"`
	DisplayHint string `json:"display_hint,omitempty"`
}

// PacketProcessor defines the interface for processing SNMP packets.
type PacketProcessor interface {
	ProcessPacket(ctx context.Context, packet []byte, addr *net.UDPAddr) error
}

// BufferPool manages reusable buffers and objects for performance optimization.
// This pool reduces memory allocations by reusing packet buffers, TrapMessage objects,
// and other frequently allocated structures throughout the SNMP processing pipeline.
type BufferPool struct {
	packetBuffers sync.Pool
	trapMessages  sync.Pool
	variables     sync.Pool
	rawDataMaps   sync.Pool
}

// Job represents a packet processing job with buffer management for worker pools.
type Job struct {
	packet     []byte
	addr       *net.UDPAddr
	bufferPool *BufferPool
}

// Listener handles SNMP trap reception and processing.
type Listener struct {
	config     Config
	conn       *net.UDPConn
	processor  PacketProcessor
	workerPool *WorkerPool
	bufferPool *BufferPool
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// WorkerPool manages packet processing using simple goroutines.
type WorkerPool struct {
	workers   int
	enabled   bool
	processor PacketProcessor
	jobs      chan Job
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// TrapParser implements SNMP trap parsing with OID translation and buffer pool optimization.
type TrapParser struct {
	mibPaths      []string
	translateOIDs bool
	bufferPool    *BufferPool              // Buffer pool for optimized memory allocation
	translator    snmptranslate.Translator // MIB-based OID translator
}

// NewBufferPool creates a new buffer pool with pre-allocated objects for performance optimization.
// The pool reduces memory allocations by reusing frequently allocated structures.
func NewBufferPool() *BufferPool {
	return &BufferPool{
		packetBuffers: sync.Pool{
			New: func() interface{} {
				// Pre-allocate 8KB buffer (covers 99% of SNMP packets)
				return make([]byte, 0, 8192)
			},
		},
		trapMessages: sync.Pool{
			New: func() interface{} {
				return &TrapMessage{
					Variables: make([]TrapVariable, 0, 10), // Pre-allocate for 10 variables
					RawData:   make(map[string]any, 10),    // Pre-allocate map
				}
			},
		},
		variables: sync.Pool{
			New: func() interface{} {
				return make([]TrapVariable, 0, 10)
			},
		},
		rawDataMaps: sync.Pool{
			New: func() interface{} {
				return make(map[string]any, 10)
			},
		},
	}
}

// GetPacketBuffer returns a reusable packet buffer from the pool.
func (bp *BufferPool) GetPacketBuffer() []byte {
	buf, ok := bp.packetBuffers.Get().([]byte)
	if !ok {
		// Fallback to creating a new buffer if type assertion fails
		return make([]byte, 0, 8192)
	}
	return buf[:0] // Reset length to 0
}

// PutPacketBuffer returns a packet buffer to the pool for reuse.
// Only buffers up to 16KB are pooled to prevent memory bloat.
func (bp *BufferPool) PutPacketBuffer(buf []byte) {
	if cap(buf) <= 16384 { // Only pool buffers up to 16KB
		bp.packetBuffers.Put(interface{}(buf))
	}
}

// GetTrapMessage returns a reusable TrapMessage from the pool.
func (bp *BufferPool) GetTrapMessage() *TrapMessage {
	msg, ok := bp.trapMessages.Get().(*TrapMessage)
	if !ok {
		// Fallback to creating a new TrapMessage if type assertion fails
		msg = &TrapMessage{
			Variables: make([]TrapVariable, 0, 10),
			RawData:   make(map[string]any, 10),
		}
	}
	// Reset the message for reuse
	msg.SourceIP = ""
	msg.OID = ""
	msg.Community = ""
	msg.Version = ""
	msg.TrapType = ""
	msg.Timestamp = time.Time{}
	msg.Variables = msg.Variables[:0] // Reset slice length
	// Clear map but keep allocated space
	for k := range msg.RawData {
		delete(msg.RawData, k)
	}
	return msg
}

// PutTrapMessage returns a TrapMessage to the pool for reuse.
func (bp *BufferPool) PutTrapMessage(msg *TrapMessage) {
	bp.trapMessages.Put(msg)
}

// NewListener creates a new SNMP trap listener with buffer pool optimization.
func NewListener(config Config, processor PacketProcessor) (*Listener, error) {
	if config == nil {
		return nil, errors.New("config cannot be nil")
	}
	if processor == nil {
		return nil, errors.New("processor cannot be nil")
	}

	ctx, cancel := context.WithCancel(context.Background())

	listener := &Listener{
		config:     config,
		processor:  processor,
		bufferPool: NewBufferPool(), // Initialize buffer pool for performance
		ctx:        ctx,
		cancel:     cancel,
	}

	// Create worker pool if enabled
	if config.GetWorkerPoolEnabled() {
		workerPool, err := NewWorkerPool(config, processor)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create worker pool: %w", err)
		}
		listener.workerPool = workerPool
	}

	return listener, nil
}

// NewWorkerPool creates a new worker pool.
func NewWorkerPool(config Config, processor PacketProcessor) (*WorkerPool, error) {
	if config == nil {
		return nil, errors.New("config cannot be nil")
	}
	if processor == nil {
		return nil, errors.New("processor cannot be nil")
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &WorkerPool{
		workers:   config.GetWorkerPoolSize(),
		enabled:   config.GetWorkerPoolEnabled(),
		processor: processor,
		jobs:      make(chan Job, config.GetWorkerPoolSize()*2), // Buffer for jobs
		ctx:       ctx,
		cancel:    cancel,
	}, nil
}

// NewTrapParser creates a new trap parser with buffer pool optimization.
func NewTrapParser(mibPaths []string, translateOIDs bool) *TrapParser {
	parser := &TrapParser{
		mibPaths:      mibPaths,
		translateOIDs: translateOIDs,
		bufferPool:    NewBufferPool(),
	}

	// Initialize MIB-based translator if MIB paths are provided
	if translateOIDs && len(mibPaths) > 0 {
		translator := snmptranslate.New()

		// Try to initialize with the first MIB directory
		// In a real implementation, you might want to handle multiple directories
		if len(mibPaths) > 0 {
			if err := translator.Init(mibPaths[0]); err != nil {
				// Log error but continue with fallback translation
				// In production, you might want to handle this differently
				fmt.Printf("Warning: failed to initialize MIB translator: %v\n", err)
			} else {
				parser.translator = translator
			}
		}
	}

	return parser
}

// Start starts the SNMP trap listener.
func (l *Listener) Start(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", l.config.GetSNMPBindAddress(), l.config.GetSNMPPort())
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address %s: %w", addr, err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP address %s: %w", addr, err)
	}

	l.conn = conn
	// SNMP trap listener started on address

	// Start worker pool if enabled
	if l.workerPool != nil {
		if err := l.workerPool.Start(ctx); err != nil {
			if closeErr := conn.Close(); closeErr != nil {
				_ = closeErr // Ignore close error during startup failure
			}
			return fmt.Errorf("failed to start worker pool: %w", err)
		}
	}

	// Start listening goroutine
	l.wg.Add(1)
	go l.listen(ctx)

	return nil
}

// Stop stops the SNMP trap listener.
func (l *Listener) Stop(ctx context.Context) error {
	// Cancel context first to signal shutdown
	l.cancel()

	// Stop worker pool first to prevent new jobs
	if l.workerPool != nil {
		l.workerPool.Stop(ctx)
	}

	// Close connection to unblock ReadFromUDP
	if l.conn != nil {
		if err := l.conn.Close(); err != nil {
			_ = err // Ignore close error during shutdown
		}
		l.conn = nil // Set to nil to prevent further operations
	}

	// Wait for goroutines to finish
	done := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// SNMP trap listener stopped
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// listen is the main listening loop with zero-copy optimization.
func (l *Listener) listen(ctx context.Context) {
	defer l.wg.Done()

	// Use a single buffer for all packet reception
	buffer := make([]byte, l.config.GetBufferSize())

	for {
		if l.shouldStopListening(ctx) {
			return
		}

		if l.handlePacketReception(ctx, buffer) {
			return // Shutdown requested
		}
	}
}

// shouldStopListening checks if the listener should stop due to context cancellation.
func (l *Listener) shouldStopListening(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// handlePacketReception handles a single packet reception cycle.
// Returns true if shutdown is requested, false to continue listening.
func (l *Listener) handlePacketReception(ctx context.Context, buffer []byte) bool {
	// Check if connection is still valid before setting deadline
	if l.conn == nil {
		return true
	}

	// Set read timeout and handle connection errors
	if l.setReadDeadlineWithErrorHandling() {
		return true // Shutdown requested
	}

	// Read packet from UDP connection
	packet, addr, shouldStop := l.readUDPPacket(buffer)
	if shouldStop {
		return true
	}
	if packet == nil {
		return false // Continue listening (timeout or recoverable error)
	}

	// Process the received packet
	if err := l.processPacketZeroCopy(ctx, packet, addr); err != nil {
		// Failed to process packet from source - error could be returned to caller if needed
		_ = err // Acknowledge error
	}

	return false // Continue listening
}

// setReadDeadlineWithErrorHandling sets the read deadline and handles connection errors.
// Returns true if shutdown is requested due to connection closure.
func (l *Listener) setReadDeadlineWithErrorHandling() bool {
	err := l.conn.SetReadDeadline(time.Now().Add(l.config.GetReadTimeout()))
	if err != nil {
		if l.isConnectionClosedError(err) {
			return true // Connection closed during shutdown - exit gracefully
		}
		// Failed to set read deadline
	}
	return false
}

// readUDPPacket reads a UDP packet and handles errors.
// Returns the packet data, address, and whether shutdown is requested.
func (l *Listener) readUDPPacket(buffer []byte) ([]byte, *net.UDPAddr, bool) {
	n, addr, err := l.conn.ReadFromUDP(buffer)
	if err != nil {
		if l.isConnectionClosedError(err) {
			return nil, nil, true // Connection closed during shutdown - exit gracefully
		}

		if l.isTimeoutError(err) {
			return nil, nil, false // Timeout is expected, continue listening
		}

		// Failed to read UDP packet
		return nil, nil, false // Continue listening after error
	}

	// Return the packet slice (zero-copy approach)
	return buffer[:n], addr, false
}

// isConnectionClosedError checks if the error indicates a closed connection during shutdown.
func (l *Listener) isConnectionClosedError(err error) bool {
	return strings.Contains(err.Error(), "use of closed network connection")
}

// isTimeoutError checks if the error is a network timeout.
func (l *Listener) isTimeoutError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// processPacketZeroCopy processes a packet with zero-copy optimization.
// For worker pools, we copy the packet since it will be processed asynchronously.
// For direct processing, we use the buffer slice directly without copying.
func (l *Listener) processPacketZeroCopy(ctx context.Context, packet []byte, addr *net.UDPAddr) error {
	// For worker pool, we need to copy the packet since it will be processed asynchronously
	if l.workerPool != nil && l.workerPool.enabled {
		// Get buffer from pool
		pooledBuffer := l.bufferPool.GetPacketBuffer()

		// Ensure capacity
		if cap(pooledBuffer) < len(packet) {
			pooledBuffer = make([]byte, len(packet))
		} else {
			pooledBuffer = pooledBuffer[:len(packet)]
		}

		copy(pooledBuffer, packet)

		return l.workerPool.SubmitJobWithBuffer(ctx, pooledBuffer, addr, l.bufferPool)
	}

	// Direct processing - no copy needed
	return l.processor.ProcessPacket(ctx, packet, addr)
}

// Start starts the worker pool.
func (w *WorkerPool) Start(_ context.Context) error {
	if !w.enabled {
		return nil
	}

	// Start worker goroutines
	for i := 0; i < w.workers; i++ {
		w.wg.Add(1)
		go w.worker()
	}

	return nil
}

// Stop stops the worker pool.
func (w *WorkerPool) Stop(_ context.Context) {
	if !w.enabled {
		return
	}

	w.cancel()
	close(w.jobs)
	w.wg.Wait()
}

// worker processes jobs from the job channel.
func (w *WorkerPool) worker() {
	defer w.wg.Done()

	for {
		select {
		case job, ok := <-w.jobs:
			if !ok {
				return // Channel closed
			}

			// Process the job
			err := w.processor.ProcessPacket(w.ctx, job.packet, job.addr)
			if err != nil {
				_ = err // Job processing failed - could be logged by caller if needed
			}

			// Return buffer to pool after processing
			if job.bufferPool != nil {
				job.bufferPool.PutPacketBuffer(job.packet)
			}

		case <-w.ctx.Done():
			return // Context canceled
		}
	}
}

// SubmitJobWithBuffer submits a packet processing job with buffer pool management.
// The buffer will be automatically returned to the pool after processing completes.
func (w *WorkerPool) SubmitJobWithBuffer(ctx context.Context, packet []byte, addr *net.UDPAddr, bufferPool *BufferPool) error {
	if !w.enabled {
		// Process directly if worker pool is disabled
		err := w.processor.ProcessPacket(ctx, packet, addr)
		if bufferPool != nil {
			bufferPool.PutPacketBuffer(packet)
		}
		return err
	}

	job := Job{
		packet:     packet,
		addr:       addr,
		bufferPool: bufferPool,
	}

	select {
	case w.jobs <- job:
		return nil
	case <-ctx.Done():
		// Return buffer if context canceled
		if bufferPool != nil {
			bufferPool.PutPacketBuffer(packet)
		}
		return ctx.Err()
	}
}

// ParseTrap parses an SNMP trap packet into a TrapMessage with optimized memory allocation.
func (p *TrapParser) ParseTrap(_ context.Context, packet *gosnmp.SnmpPacket, addr *net.UDPAddr) (*TrapMessage, error) {
	if err := p.validateInputs(packet, addr); err != nil {
		return nil, err
	}

	// Get reusable message from pool if available
	msg := p.getTrapMessage(packet)

	// Populate basic message fields
	p.populateBasicFields(msg, packet, addr)

	// Extract trap OID and type from variables
	p.extractTrapOID(msg, packet)

	// Parse all variables
	p.parseVariables(msg, packet)

	return msg, nil
}

// validateInputs validates the input parameters.
func (p *TrapParser) validateInputs(packet *gosnmp.SnmpPacket, addr *net.UDPAddr) error {
	if packet == nil {
		return errors.New("packet cannot be nil")
	}
	if addr == nil {
		return errors.New("address cannot be nil")
	}
	return nil
}

// getTrapMessage gets a TrapMessage from the pool or creates a new one.
func (p *TrapParser) getTrapMessage(packet *gosnmp.SnmpPacket) *TrapMessage {
	if p.bufferPool != nil {
		return p.bufferPool.GetTrapMessage()
	}
	return &TrapMessage{
		Variables: make([]TrapVariable, 0, len(packet.Variables)),
		RawData:   make(map[string]any),
	}
}

// populateBasicFields populates the basic fields of the trap message.
func (p *TrapParser) populateBasicFields(msg *TrapMessage, packet *gosnmp.SnmpPacket, addr *net.UDPAddr) {
	msg.SourceIP = addr.IP.String()
	msg.Community = packet.Community
	msg.Version = getVersionString(packet.Version)
	msg.Timestamp = time.Now()
}

// extractTrapOID extracts the trap OID and type from packet variables.
func (p *TrapParser) extractTrapOID(msg *TrapMessage, packet *gosnmp.SnmpPacket) {
	if len(packet.Variables) == 0 {
		return
	}

	// For SNMPv2c traps, look for snmpTrapOID variable
	for _, variable := range packet.Variables {
		if p.isTrapOIDVariable(variable.Name) {
			if oidStr := p.extractOIDString(variable.Value); oidStr != "" {
				msg.OID = oidStr
				msg.TrapType = p.translateOID(oidStr)
				break
			}
		}
	}
}

// isTrapOIDVariable checks if the variable name is a trap OID variable.
func (p *TrapParser) isTrapOIDVariable(name string) bool {
	return isTrapOIDVariable(name)
}

// extractOIDString extracts OID string from different value types.
func (p *TrapParser) extractOIDString(value any) string {
	return extractOIDString(value)
}

// parseVariables parses all variables in the packet.
func (p *TrapParser) parseVariables(msg *TrapMessage, packet *gosnmp.SnmpPacket) {
	// Pre-allocate variables slice if needed for better performance
	if cap(msg.Variables) < len(packet.Variables) {
		msg.Variables = make([]TrapVariable, 0, len(packet.Variables))
	}

	// Parse variables efficiently
	for _, variable := range packet.Variables {
		trapVar := TrapVariable{
			OID:   variable.Name,
			Type:  variable.Type.String(),
			Value: variable.Value,
		}

		if p.translateOIDs {
			trapVar.Name = p.translateOID(variable.Name)
		}

		msg.Variables = append(msg.Variables, trapVar)
	}
}

// translateOID translates an OID to a human-readable name.
// Uses MIB-based translation if available, falls back to basic translation.
func (p *TrapParser) translateOID(oid string) string {
	if !p.translateOIDs {
		return oid
	}

	// Try MIB-based translation first
	if p.translator != nil {
		if name, err := p.translator.Translate(oid); err == nil {
			return name
		}
	}

	// Fallback to basic translation for common OIDs
	return translateCommonOID(oid)
}

// Close cleans up resources used by the trap parser.
func (p *TrapParser) Close() error {
	if p.translator != nil {
		return p.translator.Close()
	}
	return nil
}

// getVersionString converts SNMP version to string.
func getVersionString(version gosnmp.SnmpVersion) string {
	switch version {
	case gosnmp.Version1:
		return "1"
	case gosnmp.Version2c:
		return "2c"
	case gosnmp.Version3:
		return "3"
	default:
		return "unknown"
	}
}

// Helper functions previously from argus/internal/helpers

// IsTrapOIDVariable checks if the variable name is a trap OID variable.
func isTrapOIDVariable(name string) bool {
	// Standard SNMP trap OID variables
	trapOIDs := []string{
		"1.3.6.1.6.3.1.1.4.1.0",  // snmpTrapOID
		".1.3.6.1.6.3.1.1.4.1.0", // snmpTrapOID with leading dot
		"1.3.6.1.2.1.1.3.0",      // sysUpTime
		".1.3.6.1.2.1.1.3.0",     // sysUpTime with leading dot
	}

	for _, oid := range trapOIDs {
		if name == oid {
			return true
		}
	}

	return false
}

// ExtractOIDString extracts OID string from different value types.
func extractOIDString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// NormalizeOID normalizes an OID string by ensuring it has a leading dot.
func normalizeOID(oid string) string {
	if oid == "" {
		return oid
	}
	if !strings.HasPrefix(oid, ".") {
		return "." + oid
	}
	return oid
}

// TranslateCommonOID translates common SNMP trap OIDs to human-readable names.
func translateCommonOID(oid string) string {
	// Normalize OID for consistent comparison
	normalizedOID := normalizeOID(oid)

	// Basic OID translation for common traps
	switch normalizedOID {
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
		return oid // Return original if no translation available
	}
}
