package serializer

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
)

// MarshalBinary serializes UserData directly into a custom flat binary format.
func MarshalBinary(u *model.UserData) ([]byte, error) {
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
func UnmarshalBinary(data []byte, u *model.UserData) error {
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
