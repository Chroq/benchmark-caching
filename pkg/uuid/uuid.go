package uuid

import (
	"crypto/rand"
	"errors"
	"fmt"
)

// NewUUID generates a cryptographically secure random UUID (Version 4).
func NewUUID() ([16]byte, error) {
	var uuid [16]byte
	_, err := rand.Read(uuid[:])
	if err != nil {
		return uuid, err
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return uuid, nil
}

// UUIDToString formats the [16]byte UUID into the standard canonical 36-character string.
func UUIDToString(uuid [16]byte) string {
	const hextable = "0123456789abcdef"
	var buf [36]byte

	buf[0] = hextable[uuid[0]>>4]
	buf[1] = hextable[uuid[0]&0x0f]
	buf[2] = hextable[uuid[1]>>4]
	buf[3] = hextable[uuid[1]&0x0f]
	buf[4] = hextable[uuid[2]>>4]
	buf[5] = hextable[uuid[2]&0x0f]
	buf[6] = hextable[uuid[3]>>4]
	buf[7] = hextable[uuid[3]&0x0f]
	buf[8] = '-'

	buf[9] = hextable[uuid[4]>>4]
	buf[10] = hextable[uuid[4]&0x0f]
	buf[11] = hextable[uuid[5]>>4]
	buf[12] = hextable[uuid[5]&0x0f]
	buf[13] = '-'

	buf[14] = hextable[uuid[6]>>4]
	buf[15] = hextable[uuid[6]&0x0f]
	buf[16] = hextable[uuid[7]>>4]
	buf[17] = hextable[uuid[7]&0x0f]
	buf[18] = '-'

	buf[19] = hextable[uuid[8]>>4]
	buf[20] = hextable[uuid[8]&0x0f]
	buf[21] = hextable[uuid[9]>>4]
	buf[22] = hextable[uuid[9]&0x0f]
	buf[23] = '-'

	buf[24] = hextable[uuid[10]>>4]
	buf[25] = hextable[uuid[10]&0x0f]
	buf[26] = hextable[uuid[11]>>4]
	buf[27] = hextable[uuid[11]&0x0f]
	buf[28] = hextable[uuid[12]>>4]
	buf[29] = hextable[uuid[12]&0x0f]
	buf[30] = hextable[uuid[13]>>4]
	buf[31] = hextable[uuid[13]&0x0f]
	buf[32] = hextable[uuid[14]>>4]
	buf[33] = hextable[uuid[14]&0x0f]
	buf[34] = hextable[uuid[15]>>4]
	buf[35] = hextable[uuid[15]&0x0f]

	return string(buf[:])
}

// ParseUUID decodes a standard 36-character UUID string back into a [16]byte array.
func ParseUUID(s string) ([16]byte, error) {
	var uuid [16]byte
	if len(s) != 36 {
		return uuid, fmt.Errorf("invalid UUID length: expected 36, got %d", len(s))
	}
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return uuid, errors.New("invalid UUID format: missing hyphens in expected positions")
	}

	decodeHex := func(b1, b2 byte) (byte, error) {
		var h1, h2 byte
		switch {
		case b1 >= '0' && b1 <= '9':
			h1 = b1 - '0'
		case b1 >= 'a' && b1 <= 'f':
			h1 = b1 - 'a' + 10
		case b1 >= 'A' && b1 <= 'F':
			h1 = b1 - 'A' + 10
		default:
			return 0, fmt.Errorf("invalid character in UUID hex: %c", b1)
		}
		switch {
		case b2 >= '0' && b2 <= '9':
			h2 = b2 - '0'
		case b2 >= 'a' && b2 <= 'f':
			h2 = b2 - 'a' + 10
		case b2 >= 'A' && b2 <= 'F':
			h2 = b2 - 'A' + 10
		default:
			return 0, fmt.Errorf("invalid character in UUID hex: %c", b2)
		}
		return (h1 << 4) | h2, nil
	}

	var err error
	src := []byte(s)

	uuid[0], err = decodeHex(src[0], src[1])
	if err != nil {
		return uuid, err
	}
	uuid[1], err = decodeHex(src[2], src[3])
	if err != nil {
		return uuid, err
	}
	uuid[2], err = decodeHex(src[4], src[5])
	if err != nil {
		return uuid, err
	}
	uuid[3], err = decodeHex(src[6], src[7])
	if err != nil {
		return uuid, err
	}

	uuid[4], err = decodeHex(src[9], src[10])
	if err != nil {
		return uuid, err
	}
	uuid[5], err = decodeHex(src[11], src[12])
	if err != nil {
		return uuid, err
	}

	uuid[6], err = decodeHex(src[14], src[15])
	if err != nil {
		return uuid, err
	}
	uuid[7], err = decodeHex(src[16], src[17])
	if err != nil {
		return uuid, err
	}

	uuid[8], err = decodeHex(src[19], src[20])
	if err != nil {
		return uuid, err
	}
	uuid[9], err = decodeHex(src[21], src[22])
	if err != nil {
		return uuid, err
	}

	uuid[10], err = decodeHex(src[24], src[25])
	if err != nil {
		return uuid, err
	}
	uuid[11], err = decodeHex(src[26], src[27])
	if err != nil {
		return uuid, err
	}
	uuid[12], err = decodeHex(src[28], src[29])
	if err != nil {
		return uuid, err
	}
	uuid[13], err = decodeHex(src[30], src[31])
	if err != nil {
		return uuid, err
	}
	uuid[14], err = decodeHex(src[32], src[33])
	if err != nil {
		return uuid, err
	}
	uuid[15], err = decodeHex(src[34], src[35])
	if err != nil {
		return uuid, err
	}

	return uuid, nil
}
