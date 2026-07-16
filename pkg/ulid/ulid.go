package ulid

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sync"
	"time"
)

// ULIDGenerator generates monotonic ULIDs.
type ULIDGenerator struct {
	mu       sync.Mutex
	lastTime uint64
	entropy  [10]byte
}

// NewULIDGenerator creates a crypto-rand seeded ULID generator.
func NewULIDGenerator() (*ULIDGenerator, error) {
	g := &ULIDGenerator{}
	if _, err := rand.Read(g.entropy[:]); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *ULIDGenerator) incEntropy() bool {
	for i := 9; i >= 0; i-- {
		g.entropy[i]++
		if g.entropy[i] != 0 {
			return true
		}
	}
	return false
}

// GenerateULID yields the next strictly monotonic [16]byte ULID.
func (g *ULIDGenerator) GenerateULID() (ulid [16]byte) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := uint64(time.Now().UnixMilli())
	if now <= g.lastTime {
		now = g.lastTime
		if !g.incEntropy() {
			g.lastTime++
			now = g.lastTime
			_, _ = rand.Read(g.entropy[:])
		}
	} else {
		g.lastTime = now
		_, _ = rand.Read(g.entropy[:])
	}

	binary.BigEndian.PutUint64(ulid[0:8], now<<16)
	copy(ulid[6:], g.entropy[:])
	return
}

// EncodeULID formats a [16]byte ULID into a canonical 36-character UUID string.
func EncodeULID(u [16]byte) string {
	const hex = "0123456789abcdef"
	var buf [36]byte

	buf[0] = hex[u[0]>>4]
	buf[1] = hex[u[0]&0x0f]
	buf[2] = hex[u[1]>>4]
	buf[3] = hex[u[1]&0x0f]
	buf[4] = hex[u[2]>>4]
	buf[5] = hex[u[2]&0x0f]
	buf[6] = hex[u[3]>>4]
	buf[7] = hex[u[3]&0x0f]
	buf[8] = '-'
	buf[9] = hex[u[4]>>4]
	buf[10] = hex[u[4]&0x0f]
	buf[11] = hex[u[5]>>4]
	buf[12] = hex[u[5]&0x0f]
	buf[13] = '-'
	buf[14] = hex[u[6]>>4]
	buf[15] = hex[u[6]&0x0f]
	buf[16] = hex[u[7]>>4]
	buf[17] = hex[u[7]&0x0f]
	buf[18] = '-'
	buf[19] = hex[u[8]>>4]
	buf[20] = hex[u[8]&0x0f]
	buf[21] = hex[u[9]>>4]
	buf[22] = hex[u[9]&0x0f]
	buf[23] = '-'
	buf[24] = hex[u[10]>>4]
	buf[25] = hex[u[10]&0x0f]
	buf[26] = hex[u[11]>>4]
	buf[27] = hex[u[11]&0x0f]
	buf[28] = hex[u[12]>>4]
	buf[29] = hex[u[12]&0x0f]
	buf[30] = hex[u[13]>>4]
	buf[31] = hex[u[13]&0x0f]
	buf[32] = hex[u[14]>>4]
	buf[33] = hex[u[14]&0x0f]
	buf[34] = hex[u[15]>>4]
	buf[35] = hex[u[15]&0x0f]

	return string(buf[:])
}

func parseHexByte(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	default:
		return 0, false
	}
}

// ParseULID decodes a 36-character UUID string back into a [16]byte ULID.
func ParseULID(s string) ([16]byte, error) {
	var u [16]byte
	if len(s) != 36 {
		return u, fmt.Errorf("invalid ULID length: expected 36, got %d", len(s))
	}
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return u, fmt.Errorf("invalid ULID format: missing hyphens")
	}

	byteIdx := 0
	for i := 0; i < 36; i += 2 {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			i++
		}
		h1, ok1 := parseHexByte(s[i])
		h2, ok2 := parseHexByte(s[i+1])
		if !ok1 || !ok2 {
			return u, fmt.Errorf("invalid hex character in ULID: %s", s)
		}
		u[byteIdx] = (h1 << 4) | h2
		byteIdx++
	}
	return u, nil
}
