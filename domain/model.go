package domain

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"
)

var CurrentTimeUTC atomic.Value
var BootstrapTime time.Time

func init() {
	CurrentTimeUTC.Store(time.Now().UTC())
}

// UserData represents the cache entry structure for the benchmark.
type UserData struct {
	ID        [16]byte `json:"id"`         // 16-byte raw UUID/ULID
	FirstName string   `json:"first_name"` // Dynamic length string
	LastName  string   `json:"last_name"`  // Dynamic length string
	BirthDate int64    `json:"birth_date"` // Unix Timestamp
	Active    bool     `json:"active"`     // Boolean
	CreatedAt int64    `json:"created_at"` // Unix Timestamp
	UpdatedAt int64    `json:"updated_at"` // Unix Timestamp
	DeletedAt int64    `json:"deleted_at"` // Unix Timestamp
}

// MarshalJSON custom fast serializer to avoid reflection and extra allocations.
func (u *UserData) MarshalJSON() ([]byte, error) {
	uuidStr := UUIDToString(u.ID)

	buf := make([]byte, 0, 150+len(u.FirstName)+len(u.LastName))
	buf = append(buf, `{"id":"`...)
	buf = append(buf, uuidStr...)
	buf = append(buf, `","first_name":"`...)
	buf = append(buf, u.FirstName...)
	buf = append(buf, `","last_name":"`...)
	buf = append(buf, u.LastName...)
	buf = append(buf, `","birth_date":`...)
	buf = strconv.AppendInt(buf, u.BirthDate, 10)
	buf = append(buf, `,"active":`...)
	if u.Active {
		buf = append(buf, "true"...)
	} else {
		buf = append(buf, "false"...)
	}
	buf = append(buf, `,"created_at":`...)
	buf = strconv.AppendInt(buf, u.CreatedAt, 10)
	buf = append(buf, `,"updated_at":`...)
	buf = strconv.AppendInt(buf, u.UpdatedAt, 10)
	buf = append(buf, `,"deleted_at":`...)
	buf = strconv.AppendInt(buf, u.DeletedAt, 10)
	buf = append(buf, '}')

	return buf, nil
}

// UnmarshalJSON custom fast deserializer to correctly map canonical UUID strings back into raw [16]byte arrays.
func (u *UserData) UnmarshalJSON(data []byte) error {
	type Alias UserData
	aux := &struct {
		ID string `json:"id"`
		*Alias
	}{
		Alias: (*Alias)(u),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	parsedID, err := ParseUUID(aux.ID)
	if err != nil {
		return err
	}
	u.ID = parsedID
	return nil
}

// ============================================================================
// PART I: ORIGINAL CUSTOM BINARY SERIALIZATION
// ============================================================================

// MarshalBinary serializes UserData directly into a custom flat binary format.
func (u *UserData) MarshalBinary() ([]byte, error) {
	lenFN := len(u.FirstName)
	lenLN := len(u.LastName)

	if lenFN > 65535 || lenLN > 65535 {
		return nil, errors.New("FirstName or LastName exceeds maximum serialization length of 65535 bytes")
	}

	size := 53 + lenFN + lenLN
	buf := make([]byte, size)

	copy(buf[0:16], u.ID[:])
	offset := 16

	binary.LittleEndian.PutUint16(buf[offset:offset+2], uint16(lenFN))
	offset += 2
	if lenFN > 0 {
		copy(buf[offset:offset+lenFN], u.FirstName)
		offset += lenFN
	}

	binary.LittleEndian.PutUint16(buf[offset:offset+2], uint16(lenLN))
	offset += 2
	if lenLN > 0 {
		copy(buf[offset:offset+lenLN], u.LastName)
		offset += lenLN
	}

	binary.LittleEndian.PutUint64(buf[offset:offset+8], uint64(u.BirthDate))
	offset += 8

	if u.Active {
		buf[offset] = 1
	} else {
		buf[offset] = 0
	}
	offset += 1

	binary.LittleEndian.PutUint64(buf[offset:offset+8], uint64(u.CreatedAt))
	offset += 8

	binary.LittleEndian.PutUint64(buf[offset:offset+8], uint64(u.UpdatedAt))
	offset += 8

	binary.LittleEndian.PutUint64(buf[offset:offset+8], uint64(u.DeletedAt))
	offset += 8

	return buf, nil
}

// UnmarshalBinary deserializes raw custom flat binary bytes back into UserData.
func (u *UserData) UnmarshalBinary(data []byte) error {
	if len(data) < 53 {
		return fmt.Errorf("payload too short for custom binary format: got %d bytes", len(data))
	}

	copy(u.ID[:], data[0:16])
	offset := 16

	lenFN := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
	offset += 2
	if offset+lenFN > len(data) {
		return errors.New("unexpected EOF reading FirstName")
	}
	if lenFN > 0 {
		u.FirstName = string(data[offset : offset+lenFN])
		offset += lenFN
	} else {
		u.FirstName = ""
	}

	if offset+2 > len(data) {
		return errors.New("unexpected EOF before LastName length")
	}
	lenLN := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
	offset += 2
	if offset+lenLN > len(data) {
		return errors.New("unexpected EOF reading LastName")
	}
	if lenLN > 0 {
		u.LastName = string(data[offset : offset+lenLN])
		offset += lenLN
	} else {
		u.LastName = ""
	}

	if offset+8 > len(data) {
		return errors.New("unexpected EOF reading BirthDate")
	}
	u.BirthDate = int64(binary.LittleEndian.Uint64(data[offset : offset+8]))
	offset += 8

	if offset+1 > len(data) {
		return errors.New("unexpected EOF reading Active")
	}
	u.Active = data[offset] != 0
	offset += 1

	if offset+8 > len(data) {
		return errors.New("unexpected EOF reading CreatedAt")
	}
	u.CreatedAt = int64(binary.LittleEndian.Uint64(data[offset : offset+8]))
	offset += 8

	if offset+8 > len(data) {
		return errors.New("unexpected EOF reading UpdatedAt")
	}
	u.UpdatedAt = int64(binary.LittleEndian.Uint64(data[offset : offset+8]))
	offset += 8

	if offset+8 > len(data) {
		return errors.New("unexpected EOF reading DeletedAt")
	}
	u.DeletedAt = int64(binary.LittleEndian.Uint64(data[offset : offset+8]))
	offset += 8

	if offset != len(data) {
		return fmt.Errorf("trailing unread data in custom binary format: read %d of %d bytes", offset, len(data))
	}

	return nil
}

// ============================================================================
// PART II: MANUAL PROTOBUF WIRE SERIALIZATION (OPTIMIZED)
// ============================================================================

// MarshalProtobuf encodes UserData into standard Protobuf wire format.
// Tag mapping:
// - ID (bytes) = 1 (key: 1<<3 | 2 = 10 / 0x0a)
// - FirstName (string) = 2 (key: 2<<3 | 2 = 18 / 0x12)
// - LastName (string) = 3 (key: 3<<3 | 2 = 26 / 0x1a)
// - BirthDate (int64) = 4 (key: 4<<3 | 0 = 32 / 0x20)
// - Active (bool) = 5 (key: 5<<3 | 0 = 40 / 0x28)
// - CreatedAt (int64) = 6 (key: 6<<3 | 0 = 48 / 0x30)
// - UpdatedAt (int64) = 7 (key: 7<<3 | 0 = 56 / 0x38)
// - DeletedAt (int64) = 8 (key: 8<<3 | 0 = 64 / 0x40)
func (u *UserData) MarshalProtobuf() ([]byte, error) {
	// Allocate a slice with a reasonable capacity to minimize re-allocations
	buf := make([]byte, 0, 64+len(u.FirstName)+len(u.LastName))

	// Tag 1: ID (bytes, wire-type 2)
	buf = append(buf, 0x0a)
	buf = appendVarint(buf, 16)
	buf = append(buf, u.ID[:]...)

	// Tag 2: FirstName (string, wire-type 2)
	if len(u.FirstName) > 0 {
		buf = append(buf, 0x12)
		buf = appendVarint(buf, uint64(len(u.FirstName)))
		buf = append(buf, u.FirstName...)
	}

	// Tag 3: LastName (string, wire-type 2)
	if len(u.LastName) > 0 {
		buf = append(buf, 0x1a)
		buf = appendVarint(buf, uint64(len(u.LastName)))
		buf = append(buf, u.LastName...)
	}

	// Tag 4: BirthDate (int64, wire-type 0)
	if u.BirthDate != 0 {
		buf = append(buf, 0x20)
		buf = appendVarint(buf, uint64(u.BirthDate))
	}

	// Tag 5: Active (bool, wire-type 0)
	if u.Active {
		buf = append(buf, 0x28)
		buf = append(buf, 1)
	}

	// Tag 6: CreatedAt (int64, wire-type 0)
	if u.CreatedAt != 0 {
		buf = append(buf, 0x30)
		buf = appendVarint(buf, uint64(u.CreatedAt))
	}

	// Tag 7: UpdatedAt (int64, wire-type 0)
	if u.UpdatedAt != 0 {
		buf = append(buf, 0x38)
		buf = appendVarint(buf, uint64(u.UpdatedAt))
	}

	// Tag 8: DeletedAt (int64, wire-type 0)
	if u.DeletedAt != 0 {
		buf = append(buf, 0x40)
		buf = appendVarint(buf, uint64(u.DeletedAt))
	}

	return buf, nil
}

// UnmarshalProtobuf parses standard Protobuf wire format into UserData.
func (u *UserData) UnmarshalProtobuf(data []byte) error {
	u.ID = [16]byte{}
	u.FirstName = ""
	u.LastName = ""
	u.BirthDate = 0
	u.Active = false
	u.CreatedAt = 0
	u.UpdatedAt = 0
	u.DeletedAt = 0

	offset := 0
	for offset < len(data) {
		key, n, err := consumeVarint(data[offset:])
		if err != nil {
			return err
		}
		offset += n

		tag := key >> 3
		wireType := key & 0x07

		switch tag {
		case 1: // ID (bytes)
			if wireType != 2 {
				return fmt.Errorf("unexpected wire type for ID: %d", wireType)
			}
			length, n, err := consumeVarint(data[offset:])
			if err != nil {
				return err
			}
			offset += n
			if offset+int(length) > len(data) {
				return errors.New("unexpected EOF reading ID payload")
			}
			if length != 16 {
				return fmt.Errorf("invalid ID length: expected 16, got %d", length)
			}
			copy(u.ID[:], data[offset:offset+16])
			offset += 16

		case 2: // FirstName (string)
			if wireType != 2 {
				return fmt.Errorf("unexpected wire type for FirstName: %d", wireType)
			}
			length, n, err := consumeVarint(data[offset:])
			if err != nil {
				return err
			}
			offset += n
			if offset+int(length) > len(data) {
				return errors.New("unexpected EOF reading FirstName payload")
			}
			u.FirstName = string(data[offset : offset+int(length)])
			offset += int(length)

		case 3: // LastName (string)
			if wireType != 2 {
				return fmt.Errorf("unexpected wire type for LastName: %d", wireType)
			}
			length, n, err := consumeVarint(data[offset:])
			if err != nil {
				return err
			}
			offset += n
			if offset+int(length) > len(data) {
				return errors.New("unexpected EOF reading LastName payload")
			}
			u.LastName = string(data[offset : offset+int(length)])
			offset += int(length)

		case 4: // BirthDate (int64 varint)
			if wireType != 0 {
				return fmt.Errorf("unexpected wire type for BirthDate: %d", wireType)
			}
			val, n, err := consumeVarint(data[offset:])
			if err != nil {
				return err
			}
			offset += n
			u.BirthDate = int64(val)

		case 5: // Active (bool varint)
			if wireType != 0 {
				return fmt.Errorf("unexpected wire type for Active: %d", wireType)
			}
			val, n, err := consumeVarint(data[offset:])
			if err != nil {
				return err
			}
			offset += n
			u.Active = val != 0

		case 6: // CreatedAt (int64 varint)
			if wireType != 0 {
				return fmt.Errorf("unexpected wire type for CreatedAt: %d", wireType)
			}
			val, n, err := consumeVarint(data[offset:])
			if err != nil {
				return err
			}
			offset += n
			u.CreatedAt = int64(val)

		case 7: // UpdatedAt (int64 varint)
			if wireType != 0 {
				return fmt.Errorf("unexpected wire type for UpdatedAt: %d", wireType)
			}
			val, n, err := consumeVarint(data[offset:])
			if err != nil {
				return err
			}
			offset += n
			u.UpdatedAt = int64(val)

		case 8: // DeletedAt (int64 varint)
			if wireType != 0 {
				return fmt.Errorf("unexpected wire type for DeletedAt: %d", wireType)
			}
			val, n, err := consumeVarint(data[offset:])
			if err != nil {
				return err
			}
			offset += n
			u.DeletedAt = int64(val)

		default:
			// Skip unknown fields (standard protobuf behavior)
			switch wireType {
			case 0: // Varint
				_, n, err := consumeVarint(data[offset:])
				if err != nil {
					return err
				}
				offset += n
			case 1: // 64-bit
				if offset+8 > len(data) {
					return errors.New("unexpected EOF skipping 64-bit field")
				}
				offset += 8
			case 2: // Length-delimited
				length, n, err := consumeVarint(data[offset:])
				if err != nil {
					return err
				}
				offset += n
				if offset+int(length) > len(data) {
					return errors.New("unexpected EOF skipping length-delimited field")
				}
				offset += int(length)
			case 5: // 32-bit
				if offset+4 > len(data) {
					return errors.New("unexpected EOF skipping 32-bit field")
				}
				offset += 4
			default:
				return fmt.Errorf("unsupported protobuf wire type: %d", wireType)
			}
		}
	}
	return nil
}

// Helper to append a varint encoded uint64 to a slice
func appendVarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v|0x80))
		v >>= 7
	}
	return append(buf, byte(v))
}

// Helper to consume a varint encoded uint64 from a slice
func consumeVarint(data []byte) (uint64, int, error) {
	var v uint64
	var shift uint
	for i, b := range data {
		if i >= 10 { // Maximum varint encoding size for 64-bit integer is 10 bytes
			return 0, 0, errors.New("varint64 overflow")
		}
		v |= uint64(b&0x7f) << shift
		if b < 0x80 {
			return v, i + 1, nil
		}
		shift += 7
	}
	return 0, 0, errors.New("unexpected EOF reading varint")
}

// ============================================================================
// PART III: ZERO-LIB ULID & UUID HELPERS
// ============================================================================

// NewULID generates a cryptographically secure random ULID.
// ULID contains a 48-bit timestamp (millis since Unix epoch) followed by 80-bit random bytes.
func NewULID() ([16]byte, error) {
	var ulid [16]byte
	ms := time.Now().UnixMilli()

	// 48-bit timestamp (Big Endian)
	ulid[0] = byte(ms >> 40)
	ulid[1] = byte(ms >> 32)
	ulid[2] = byte(ms >> 24)
	ulid[3] = byte(ms >> 16)
	ulid[4] = byte(ms >> 8)
	ulid[5] = byte(ms)

	// 80-bit entropy (10 bytes)
	_, err := rand.Read(ulid[6:])
	if err != nil {
		return ulid, err
	}

	return ulid, nil
}

// EncodeULID formats a [16]byte ULID into a Crockford's Base32 string (26 characters).
func EncodeULID(u [16]byte) string {
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	var dst [26]byte

	dst[0] = alphabet[(u[0]&0xE0)>>5]
	dst[1] = alphabet[u[0]&0x1F]
	dst[2] = alphabet[(u[1]&0xF8)>>3]
	dst[3] = alphabet[((u[1]&0x07)<<2)|((u[2]&0xC0)>>6)]
	dst[4] = alphabet[(u[2]&0x3E)>>1]
	dst[5] = alphabet[((u[2]&0x01)<<4)|((u[3]&0xF0)>>4)]
	dst[6] = alphabet[((u[3]&0x0F)<<1)|((u[4]&0x80)>>7)]
	dst[7] = alphabet[(u[4]&0x7C)>>2]
	dst[8] = alphabet[((u[4]&0x03)<<3)|((u[5]&0xE0)>>5)]
	dst[9] = alphabet[u[5]&0x1F]

	dst[10] = alphabet[(u[6]&0xF8)>>3]
	dst[11] = alphabet[((u[6]&0x07)<<2)|((u[7]&0xC0)>>6)]
	dst[12] = alphabet[(u[7]&0x3E)>>1]
	dst[13] = alphabet[((u[7]&0x01)<<4)|((u[8]&0xF0)>>4)]
	dst[14] = alphabet[((u[8]&0x0F)<<1)|((u[9]&0x80)>>7)]
	dst[15] = alphabet[(u[9]&0x7C)>>2]
	dst[16] = alphabet[((u[9]&0x03)<<3)|((u[10]&0xE0)>>5)]
	dst[17] = alphabet[u[10]&0x1F]

	dst[18] = alphabet[(u[11]&0xF8)>>3]
	dst[19] = alphabet[((u[11]&0x07)<<2)|((u[12]&0xC0)>>6)]
	dst[20] = alphabet[(u[12]&0x3E)>>1]
	dst[21] = alphabet[((u[12]&0x01)<<4)|((u[13]&0xF0)>>4)]
	dst[22] = alphabet[((u[13]&0x0F)<<1)|((u[14]&0x80)>>7)]
	dst[23] = alphabet[(u[14]&0x7C)>>2]
	dst[24] = alphabet[((u[14]&0x03)<<3)|((u[15]&0xE0)>>5)]
	dst[25] = alphabet[u[15]&0x1F]

	return string(dst[:])
}

// Helper to decode a single Crockford Base32 character.
func decodeCrockford(ch byte) (byte, error) {
	switch ch {
	case '0':
		return 0, nil
	case '1':
		return 1, nil
	case '2':
		return 2, nil
	case '3':
		return 3, nil
	case '4':
		return 4, nil
	case '5':
		return 5, nil
	case '6':
		return 6, nil
	case '7':
		return 7, nil
	case '8':
		return 8, nil
	case '9':
		return 9, nil
	case 'A', 'a':
		return 10, nil
	case 'B', 'b':
		return 11, nil
	case 'C', 'c':
		return 12, nil
	case 'D', 'd':
		return 13, nil
	case 'E', 'e':
		return 14, nil
	case 'F', 'f':
		return 15, nil
	case 'G', 'g':
		return 16, nil
	case 'H', 'h':
		return 17, nil
	case 'J', 'j':
		return 18, nil
	case 'K', 'k':
		return 19, nil
	case 'M', 'm':
		return 20, nil
	case 'N', 'n':
		return 21, nil
	case 'P', 'p':
		return 22, nil
	case 'Q', 'q':
		return 23, nil
	case 'R', 'r':
		return 24, nil
	case 'S', 's':
		return 25, nil
	case 'T', 't':
		return 26, nil
	case 'V', 'v':
		return 27, nil
	case 'W', 'w':
		return 28, nil
	case 'X', 'x':
		return 29, nil
	case 'Y', 'y':
		return 30, nil
	case 'Z', 'z':
		return 31, nil
	default:
		return 0, fmt.Errorf("invalid Crockford base32 character: %c", ch)
	}
}

// ParseULID decodes a Crockford's Base32 string (26 characters) back into a [16]byte ULID.
func ParseULID(s string) ([16]byte, error) {
	var u [16]byte
	if len(s) != 26 {
		return u, fmt.Errorf("invalid ULID length: expected 26, got %d", len(s))
	}

	var dec [26]byte
	var err error
	for i := 0; i < 26; i++ {
		dec[i], err = decodeCrockford(s[i])
		if err != nil {
			return u, err
		}
	}

	u[0] = (dec[0] << 5) | dec[1]
	u[1] = (dec[2] << 3) | (dec[3] >> 2)
	u[2] = (dec[3] << 6) | (dec[4] << 1) | (dec[5] >> 4)
	u[3] = (dec[5] << 4) | (dec[6] >> 1)
	u[4] = (dec[6] << 7) | (dec[7] << 2) | (dec[8] >> 3)
	u[5] = (dec[8] << 5) | dec[9]
	
	u[6] = (dec[10] << 3) | (dec[11] >> 2)
	u[7] = (dec[11] << 6) | (dec[12] << 1) | (dec[13] >> 4)
	u[8] = (dec[13] << 4) | (dec[14] >> 1)
	u[9] = (dec[14] << 7) | (dec[15] << 2) | (dec[16] >> 3)
	u[10] = (dec[16] << 5) | dec[17]
	
	u[11] = (dec[18] << 3) | (dec[19] >> 2)
	u[12] = (dec[19] << 6) | (dec[20] << 1) | (dec[21] >> 4)
	u[13] = (dec[21] << 4) | (dec[22] >> 1)
	u[14] = (dec[22] << 7) | (dec[23] << 2) | (dec[24] >> 3)
	u[15] = (dec[24] << 5) | dec[25]

	return u, nil
}

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

// Reset clears all fields of UserData to support sync.Pool recycling.
func (u *UserData) Reset() {
	u.ID = [16]byte{}
	u.FirstName = ""
	u.LastName = ""
	u.BirthDate = 0
	u.Active = false
	u.CreatedAt = 0
	u.UpdatedAt = 0
	u.DeletedAt = 0
}

