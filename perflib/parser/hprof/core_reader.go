package hprof

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

// Reader provides buffered reading of HPROF binary data.
type Reader struct {
	r       *bufio.Reader
	idSize  int
	byteBuf []byte
}

// NewReader creates a new HPROF reader.
func NewReader(r io.Reader) *Reader {
	return &Reader{
		r:       bufio.NewReaderSize(r, 64*1024), // 64KB buffer
		idSize:  8,                                // Default to 8, will be set from header
		byteBuf: make([]byte, 8),
	}
}

// SetIDSize sets the identifier size (4 or 8 bytes).
func (r *Reader) SetIDSize(size int) {
	r.idSize = size
}

// IDSize returns the current identifier size.
func (r *Reader) IDSize() int {
	return r.idSize
}

// ReadHeader reads the HPROF file header.
func (r *Reader) ReadHeader() (*Header, error) {
	// Read format string (null-terminated)
	format, err := r.readNullTerminatedString()
	if err != nil {
		return nil, fmt.Errorf("failed to read format string: %w", err)
	}

	// Read ID size (4 bytes)
	idSize, err := r.ReadUint32()
	if err != nil {
		return nil, fmt.Errorf("failed to read ID size: %w", err)
	}
	r.idSize = int(idSize)

	// Read timestamp (8 bytes, milliseconds since epoch)
	timestamp, err := r.ReadUint64()
	if err != nil {
		return nil, fmt.Errorf("failed to read timestamp: %w", err)
	}

	return &Header{
		Format:    format,
		IDSize:    int(idSize),
		Timestamp: time.UnixMilli(int64(timestamp)),
	}, nil
}

// ReadRecordHeader reads a record header (tag, time delta, length).
func (r *Reader) ReadRecordHeader() (tag RecordTag, timeDelta uint32, length uint32, err error) {
	tagByte, err := r.ReadByte()
	if err != nil {
		return 0, 0, 0, err
	}
	tag = RecordTag(tagByte)

	timeDelta, err = r.ReadUint32()
	if err != nil {
		return 0, 0, 0, err
	}

	length, err = r.ReadUint32()
	if err != nil {
		return 0, 0, 0, err
	}

	return tag, timeDelta, length, nil
}

// ReadByte reads a single byte.
func (r *Reader) ReadByte() (byte, error) {
	return r.r.ReadByte()
}

// ReadBytes reads n bytes into a new slice.
func (r *Reader) ReadBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	_, err := io.ReadFull(r.r, buf)
	return buf, err
}

// ReadUint16 reads a big-endian uint16.
func (r *Reader) ReadUint16() (uint16, error) {
	_, err := io.ReadFull(r.r, r.byteBuf[:2])
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(r.byteBuf[:2]), nil
}

// ReadUint32 reads a big-endian uint32.
func (r *Reader) ReadUint32() (uint32, error) {
	_, err := io.ReadFull(r.r, r.byteBuf[:4])
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(r.byteBuf[:4]), nil
}

// ReadUint64 reads a big-endian uint64.
func (r *Reader) ReadUint64() (uint64, error) {
	_, err := io.ReadFull(r.r, r.byteBuf[:8])
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(r.byteBuf[:8]), nil
}

// ReadID reads an identifier (size depends on header).
func (r *Reader) ReadID() (uint64, error) {
	if r.idSize == 4 {
		v, err := r.ReadUint32()
		return uint64(v), err
	}
	return r.ReadUint64()
}

// Skip skips n bytes.
func (r *Reader) Skip(n int64) error {
	_, err := r.r.Discard(int(n))
	return err
}

// readNullTerminatedString reads a null-terminated string.
func (r *Reader) readNullTerminatedString() (string, error) {
	var result []byte
	for {
		b, err := r.r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == 0 {
			break
		}
		result = append(result, b)
	}
	return string(result), nil
}

// ReadValue reads a value of the given basic type.
func (r *Reader) ReadValue(t BasicType) (interface{}, error) {
	switch t {
	case TypeBoolean, TypeByte:
		return r.ReadByte()
	case TypeChar, TypeShort:
		return r.ReadUint16()
	case TypeFloat, TypeInt:
		return r.ReadUint32()
	case TypeDouble, TypeLong:
		return r.ReadUint64()
	case TypeObject:
		return r.ReadID()
	default:
		return nil, fmt.Errorf("unknown basic type: %d", t)
	}
}
