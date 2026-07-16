package serializer

import (
	"errors"
	"fmt"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
)

// MarshalProtobuf encodes UserData into standard Protobuf wire format.
func MarshalProtobuf(u *model.UserData) ([]byte, error) {
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
func UnmarshalProtobuf(data []byte, u *model.UserData) error {
	u.Reset()

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
			switch wireType {
			case 0:
				_, n, err := consumeVarint(data[offset:])
				if err != nil {
					return err
				}
				offset += n
			case 1:
				if offset+8 > len(data) {
					return errors.New("unexpected EOF skipping 64-bit field")
				}
				offset += 8
			case 2:
				length, n, err := consumeVarint(data[offset:])
				if err != nil {
					return err
				}
				offset += n
				if offset+int(length) > len(data) {
					return errors.New("unexpected EOF skipping length-delimited field")
				}
				offset += int(length)
			case 5:
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

func appendVarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v|0x80))
		v >>= 7
	}
	return append(buf, byte(v))
}

func consumeVarint(data []byte) (uint64, int, error) {
	var v uint64
	var shift uint
	for i, b := range data {
		if i >= 10 {
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
