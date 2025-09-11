package trapprocessor

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
)

// mockProcessor implements PacketProcessor for testing
type mockProcessor struct{}

func (m *mockProcessor) ProcessPacket(ctx context.Context, packet []byte, addr *net.UDPAddr) error {
	return nil
}

// newMockConfig creates a mock configuration for testing
func newMockConfig() *configImpl {
	return &configImpl{
		snmpPort:          1162,
		snmpBindAddress:   "127.0.0.1",
		snmpVersion:       "2c",
		snmpCommunity:     "public",
		snmpMIBPaths:      []string{},
		workerPoolSize:    10,
		workerPoolEnabled: true,
		maxRetries:        3,
		bufferSize:        65536,
		readTimeout:       5 * time.Second,
		writeTimeout:      5 * time.Second,
	}
}

func TestNew(t *testing.T) {
	t.Run("empty_map_config", func(t *testing.T) {
		config := map[string]any{}
		processor, err := New(config)
		assert.NoError(t, err)
		assert.NotNil(t, processor)
	})

	t.Run("nested_map_config", func(t *testing.T) {
		config := map[string]any{
			"snmp": map[string]any{
				"port":    1162,
				"address": "0.0.0.0",
			},
			"worker_pool": map[string]any{
				"enabled": true,
				"size":    5,
			},
		}
		processor, err := New(config)
		assert.NoError(t, err)
		assert.NotNil(t, processor)
	})

	t.Run("flat_map_config", func(t *testing.T) {
		config := map[string]any{
			"snmp_port":           1162,
			"snmp_bind_address":   "0.0.0.0",
			"worker_pool_enabled": true,
			"worker_pool_size":    5,
		}
		processor, err := New(config)
		assert.NoError(t, err)
		assert.NotNil(t, processor)
	})

	t.Run("invalid_port", func(t *testing.T) {
		config := map[string]any{
			"snmp_port": -1,
		}
		processor, err := New(config)
		assert.Error(t, err)
		assert.Nil(t, processor)
	})

	t.Run("invalid_SNMP_version", func(t *testing.T) {
		config := map[string]any{
			"snmp_version": "invalid",
		}
		processor, err := New(config)
		assert.Error(t, err)
		assert.Nil(t, processor)
	})
}

func TestConfigDefaults(t *testing.T) {
	config := newMockConfig()

	assert.Equal(t, 1162, config.GetSNMPPort())
	assert.Equal(t, "127.0.0.1", config.GetSNMPBindAddress())
	assert.Equal(t, "2c", config.GetSNMPVersion())
	assert.Equal(t, "public", config.GetSNMPCommunity())
	assert.Equal(t, 10, config.GetWorkerPoolSize())
	assert.True(t, config.GetWorkerPoolEnabled())
	assert.Equal(t, 3, config.GetMaxRetries())
	assert.Equal(t, 65536, config.GetBufferSize())
	assert.Equal(t, 5*time.Second, config.GetReadTimeout())
	assert.Equal(t, 5*time.Second, config.GetWriteTimeout())
}

func TestConfigOverrides(t *testing.T) {
	config := &configImpl{
		snmpPort:          2162,
		snmpBindAddress:   "192.168.1.1",
		snmpVersion:       "1",
		snmpCommunity:     "private",
		workerPoolSize:    20,
		workerPoolEnabled: false,
		maxRetries:        5,
		bufferSize:        32768,
		readTimeout:       10 * time.Second,
		writeTimeout:      10 * time.Second,
	}

	assert.Equal(t, 2162, config.GetSNMPPort())
	assert.Equal(t, "192.168.1.1", config.GetSNMPBindAddress())
	assert.Equal(t, "1", config.GetSNMPVersion())
	assert.Equal(t, "private", config.GetSNMPCommunity())
	assert.Equal(t, 20, config.GetWorkerPoolSize())
	assert.False(t, config.GetWorkerPoolEnabled())
	assert.Equal(t, 5, config.GetMaxRetries())
	assert.Equal(t, 32768, config.GetBufferSize())
	assert.Equal(t, 10*time.Second, config.GetReadTimeout())
	assert.Equal(t, 10*time.Second, config.GetWriteTimeout())
}

func TestNewListener(t *testing.T) {
	config := newMockConfig()
	processor := &mockProcessor{}

	t.Run("valid_config_and_processor", func(t *testing.T) {
		listener, err := NewListener(config, processor)
		assert.NoError(t, err)
		assert.NotNil(t, listener)
	})

	t.Run("nil_config", func(t *testing.T) {
		listener, err := NewListener(nil, processor)
		assert.Error(t, err)
		assert.Nil(t, listener)
	})

	t.Run("nil_processor", func(t *testing.T) {
		listener, err := NewListener(config, nil)
		assert.Error(t, err)
		assert.Nil(t, listener)
	})
}

func TestNewWorkerPool(t *testing.T) {
	config := newMockConfig()
	processor := &mockProcessor{}

	t.Run("valid_config_and_processor", func(t *testing.T) {
		pool, err := NewWorkerPool(config, processor)
		assert.NoError(t, err)
		assert.NotNil(t, pool)
	})

	t.Run("disabled_worker_pool", func(t *testing.T) {
		config.workerPoolEnabled = false
		pool, err := NewWorkerPool(config, processor)
		assert.NoError(t, err)
		assert.NotNil(t, pool)
		assert.False(t, pool.enabled)
	})

	t.Run("nil_config", func(t *testing.T) {
		pool, err := NewWorkerPool(nil, processor)
		assert.Error(t, err)
		assert.Nil(t, pool)
	})

	t.Run("nil_processor", func(t *testing.T) {
		pool, err := NewWorkerPool(config, nil)
		assert.Error(t, err)
		assert.Nil(t, pool)
	})
}

func TestNewTrapParser(t *testing.T) {
	t.Run("with_mib_paths", func(t *testing.T) {
		mibPaths := []string{"/usr/share/snmp/mibs", "/etc/snmp/mibs"}
		parser := NewTrapParser(mibPaths, true)
		assert.NotNil(t, parser)
		assert.Equal(t, mibPaths, parser.mibPaths)
		assert.True(t, parser.translateOIDs)
	})

	t.Run("without_mib_paths", func(t *testing.T) {
		parser := NewTrapParser(nil, false)
		assert.NotNil(t, parser)
		assert.Nil(t, parser.mibPaths)
		assert.False(t, parser.translateOIDs)
	})
}

func TestTrapParserParseTrap(t *testing.T) {
	parser := NewTrapParser(nil, false)
	ctx := context.Background()

	t.Run("nil_packet", func(t *testing.T) {
		addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 1162}
		msg, err := parser.ParseTrap(ctx, nil, addr)
		assert.Error(t, err)
		assert.Nil(t, msg)
	})

	t.Run("nil_address", func(t *testing.T) {
		packet := &gosnmp.SnmpPacket{Version: gosnmp.Version2c}
		msg, err := parser.ParseTrap(ctx, packet, nil)
		assert.Error(t, err)
		assert.Nil(t, msg)
	})

	t.Run("valid_packet", func(t *testing.T) {
		packet := &gosnmp.SnmpPacket{
			Version:   gosnmp.Version2c,
			Community: "public",
			PDUType:   gosnmp.SNMPv2Trap,
			Variables: []gosnmp.SnmpPDU{
				{
					Name:  "1.3.6.1.2.1.1.3.0",
					Type:  gosnmp.TimeTicks,
					Value: uint32(12345),
				},
			},
		}
		addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 1162}
		msg, err := parser.ParseTrap(ctx, packet, addr)
		assert.NoError(t, err)
		assert.NotNil(t, msg)
		assert.Equal(t, "192.168.1.100", msg.SourceIP)
		assert.Equal(t, "2c", msg.Version)
		assert.Equal(t, "public", msg.Community)
		assert.Len(t, msg.Variables, 1)
	})
}
