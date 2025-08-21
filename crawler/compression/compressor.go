package compression

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/vmihailenco/msgpack/v5"
)

// CompressionType defines the type of compression
type CompressionType uint8

const (
	CompressionNone CompressionType = iota
	CompressionGzip
	CompressionZlib
	CompressionZstd
	CompressionMsgpack
	CompressionMsgpackZstd // MsgPack + Zstd for best compression
)

// CompressionLevel defines compression level
type CompressionLevel int

const (
	CompressionFastest CompressionLevel = 1
	CompressionDefault CompressionLevel = 5
	CompressionBest    CompressionLevel = 9
)

// Compressor handles data compression
type Compressor struct {
	compressionType  CompressionType
	compressionLevel CompressionLevel
	zstdEncoder      *zstd.Encoder
	zstdDecoder      *zstd.Decoder
}

// NewCompressor creates a new compressor
func NewCompressor(cType CompressionType, level CompressionLevel) (*Compressor, error) {
	c := &Compressor{
		compressionType:  cType,
		compressionLevel: level,
	}

	// Initialize Zstd encoder/decoder if needed
	if cType == CompressionZstd || cType == CompressionMsgpackZstd {
		encoder, err := zstd.NewWriter(nil, 
			zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(int(level))))
		if err != nil {
			return nil, fmt.Errorf("failed to create zstd encoder: %w", err)
		}
		c.zstdEncoder = encoder

		decoder, err := zstd.NewReader(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create zstd decoder: %w", err)
		}
		c.zstdDecoder = decoder
	}

	return c, nil
}

// Compress compresses data based on the compression type
func (c *Compressor) Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	// Add header with compression type
	result := &bytes.Buffer{}
	result.WriteByte(byte(c.compressionType))

	switch c.compressionType {
	case CompressionNone:
		result.Write(data)
		return result.Bytes(), nil

	case CompressionGzip:
		return c.compressGzip(data, result)

	case CompressionZlib:
		return c.compressZlib(data, result)

	case CompressionZstd:
		return c.compressZstd(data, result)

	case CompressionMsgpack:
		return c.compressMsgpack(data, result)

	case CompressionMsgpackZstd:
		return c.compressMsgpackZstd(data, result)

	default:
		return nil, fmt.Errorf("unsupported compression type: %d", c.compressionType)
	}
}

// CompressJSON compresses JSON data with optimization
func (c *Compressor) CompressJSON(v interface{}) ([]byte, error) {
	// For MsgPack types, directly serialize
	if c.compressionType == CompressionMsgpack || c.compressionType == CompressionMsgpackZstd {
		data, err := msgpack.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("msgpack marshal failed: %w", err)
		}
		
		if c.compressionType == CompressionMsgpackZstd {
			// Further compress with Zstd
			result := &bytes.Buffer{}
			result.WriteByte(byte(c.compressionType))
			compressed := c.zstdEncoder.EncodeAll(data, nil)
			result.Write(compressed)
			return result.Bytes(), nil
		}
		
		result := &bytes.Buffer{}
		result.WriteByte(byte(c.compressionType))
		result.Write(data)
		return result.Bytes(), nil
	}

	// For other types, use JSON
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("json marshal failed: %w", err)
	}

	return c.Compress(data)
}

// Decompress decompresses data
func (c *Compressor) Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	// Read compression type from header
	compressionType := CompressionType(data[0])
	data = data[1:]

	switch compressionType {
	case CompressionNone:
		return data, nil

	case CompressionGzip:
		return c.decompressGzip(data)

	case CompressionZlib:
		return c.decompressZlib(data)

	case CompressionZstd:
		return c.decompressZstd(data)

	case CompressionMsgpack:
		return data, nil // Already in msgpack format

	case CompressionMsgpackZstd:
		return c.decompressZstd(data)

	default:
		return nil, fmt.Errorf("unsupported compression type: %d", compressionType)
	}
}

// DecompressJSON decompresses and unmarshals JSON data
func (c *Compressor) DecompressJSON(data []byte, v interface{}) error {
	if len(data) == 0 {
		return fmt.Errorf("empty data")
	}

	compressionType := CompressionType(data[0])
	data = data[1:]

	// Handle MsgPack formats
	if compressionType == CompressionMsgpack {
		return msgpack.Unmarshal(data, v)
	}

	if compressionType == CompressionMsgpackZstd {
		decompressed, err := c.decompressZstd(data)
		if err != nil {
			return err
		}
		return msgpack.Unmarshal(decompressed, v)
	}

	// Handle JSON formats
	decompressed, err := c.Decompress(append([]byte{byte(compressionType)}, data...))
	if err != nil {
		return err
	}

	return json.Unmarshal(decompressed, v)
}

// compressGzip compresses using gzip
func (c *Compressor) compressGzip(data []byte, result *bytes.Buffer) ([]byte, error) {
	writer, err := gzip.NewWriterLevel(result, int(c.compressionLevel))
	if err != nil {
		return nil, err
	}
	defer writer.Close()

	if _, err := writer.Write(data); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return result.Bytes(), nil
}

// decompressGzip decompresses gzip data
func (c *Compressor) decompressGzip(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

// compressZlib compresses using zlib
func (c *Compressor) compressZlib(data []byte, result *bytes.Buffer) ([]byte, error) {
	writer, err := zlib.NewWriterLevel(result, int(c.compressionLevel))
	if err != nil {
		return nil, err
	}
	defer writer.Close()

	if _, err := writer.Write(data); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return result.Bytes(), nil
}

// decompressZlib decompresses zlib data
func (c *Compressor) decompressZlib(data []byte) ([]byte, error) {
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

// compressZstd compresses using zstd
func (c *Compressor) compressZstd(data []byte, result *bytes.Buffer) ([]byte, error) {
	compressed := c.zstdEncoder.EncodeAll(data, nil)
	result.Write(compressed)
	return result.Bytes(), nil
}

// decompressZstd decompresses zstd data
func (c *Compressor) decompressZstd(data []byte) ([]byte, error) {
	return c.zstdDecoder.DecodeAll(data, nil)
}

// compressMsgpack compresses using msgpack
func (c *Compressor) compressMsgpack(data []byte, result *bytes.Buffer) ([]byte, error) {
	// If it's JSON, we need to unmarshal and re-marshal as msgpack
	var obj interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		// Not JSON, store as-is
		result.Write(data)
		return result.Bytes(), nil
	}

	msgpackData, err := msgpack.Marshal(obj)
	if err != nil {
		return nil, err
	}

	result.Write(msgpackData)
	return result.Bytes(), nil
}

// compressMsgpackZstd compresses using msgpack + zstd
func (c *Compressor) compressMsgpackZstd(data []byte, result *bytes.Buffer) ([]byte, error) {
	// First convert to msgpack
	var obj interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		// Not JSON, compress as-is
		compressed := c.zstdEncoder.EncodeAll(data, nil)
		result.Write(compressed)
		return result.Bytes(), nil
	}

	msgpackData, err := msgpack.Marshal(obj)
	if err != nil {
		return nil, err
	}

	// Then compress with zstd
	compressed := c.zstdEncoder.EncodeAll(msgpackData, nil)
	result.Write(compressed)
	return result.Bytes(), nil
}

// EstimateCompressionRatio estimates the compression ratio for different types
func EstimateCompressionRatio(data []byte) map[string]float64 {
	ratios := make(map[string]float64)
	originalSize := float64(len(data))

	// Test each compression type
	types := []struct {
		name string
		ct   CompressionType
	}{
		{"gzip", CompressionGzip},
		{"zlib", CompressionZlib},
		{"zstd", CompressionZstd},
		{"msgpack", CompressionMsgpack},
		{"msgpack+zstd", CompressionMsgpackZstd},
	}

	for _, t := range types {
		c, err := NewCompressor(t.ct, CompressionDefault)
		if err != nil {
			continue
		}

		compressed, err := c.Compress(data)
		if err != nil {
			continue
		}

		ratio := (1 - float64(len(compressed))/originalSize) * 100
		ratios[t.name] = ratio
	}

	return ratios
}

// OptimizeJSONKeys optimizes JSON by shortening keys
func OptimizeJSONKeys(data []byte) ([]byte, map[string]string, error) {
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return data, nil, err
	}

	keyMap := make(map[string]string)
	optimized := optimizeObject(obj, keyMap, "")

	result, err := json.Marshal(optimized)
	return result, keyMap, err
}

// optimizeObject recursively optimizes object keys
func optimizeObject(obj interface{}, keyMap map[string]string, prefix string) interface{} {
	switch v := obj.(type) {
	case map[string]interface{}:
		newObj := make(map[string]interface{})
		for key, value := range v {
			shortKey := generateShortKey(key, keyMap)
			fullKey := prefix + key
			keyMap[fullKey] = shortKey
			newObj[shortKey] = optimizeObject(value, keyMap, fullKey+".")
		}
		return newObj

	case []interface{}:
		newArr := make([]interface{}, len(v))
		for i, item := range v {
			newArr[i] = optimizeObject(item, keyMap, prefix)
		}
		return newArr

	default:
		return v
	}
}

// generateShortKey generates a short key for optimization
func generateShortKey(key string, existing map[string]string) string {
	// Common key shortcuts
	shortcuts := map[string]string{
		"task_id":     "tid",
		"venue_id":    "vid",
		"venue_name":  "vn",
		"time_slot":   "ts",
		"status":      "s",
		"price":       "p",
		"available":   "a",
		"capacity":    "c",
		"created_at":  "ca",
		"updated_at":  "ua",
		"source":      "src",
		"metadata":    "md",
		"error":       "e",
		"result":      "r",
		"data":        "d",
	}

	if short, ok := shortcuts[strings.ToLower(key)]; ok {
		return short
	}

	// Generate based on key length
	if len(key) <= 3 {
		return key
	}

	// Use first letter and position
	return fmt.Sprintf("%c%d", key[0], len(existing))
}

// CompressedData represents compressed data with metadata
type CompressedData struct {
	Type         CompressionType `msgpack:"t"`
	OriginalSize uint32          `msgpack:"os"`
	Compressed   []byte          `msgpack:"c"`
	Checksum     uint32          `msgpack:"cs,omitempty"`
}

// WrapCompressed wraps compressed data with metadata
func WrapCompressed(data []byte, cType CompressionType, originalSize int) *CompressedData {
	return &CompressedData{
		Type:         cType,
		OriginalSize: uint32(originalSize),
		Compressed:   data,
		Checksum:     calculateChecksum(data),
	}
}

// calculateChecksum calculates a simple checksum
func calculateChecksum(data []byte) uint32 {
	var sum uint32
	for i := 0; i < len(data); i += 4 {
		if i+4 <= len(data) {
			sum += binary.LittleEndian.Uint32(data[i : i+4])
		} else {
			// Handle remaining bytes
			remaining := make([]byte, 4)
			copy(remaining, data[i:])
			sum += binary.LittleEndian.Uint32(remaining)
		}
	}
	return sum
}