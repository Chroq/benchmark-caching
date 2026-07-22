package serializer

import (
	"github.com/Chroq/benchmark-caching/internal/domain/model"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/serializer/pb"
)

// MarshalProtobuf encodes UserData into standard Protobuf wire format using VTProto (Zero-Alloc).
func MarshalProtobuf(u *model.UserData) ([]byte, error) {
	pbUser := pb.UserDataProto{
		Id:        u.ID[:],
		FirstName: u.FirstName,
		LastName:  u.LastName,
		BirthDate: u.BirthDate,
		Active:    u.Active,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
		DeletedAt: u.DeletedAt,
	}

	return pbUser.MarshalVT()
}

// UnmarshalProtobuf parses standard Protobuf wire format into UserData using VTProto (Zero-Alloc).
func UnmarshalProtobuf(data []byte, u *model.UserData) error {
	var pbUser pb.UserDataProto
	if err := pbUser.UnmarshalVT(data); err != nil {
		return err
	}

	u.Reset()
	if len(pbUser.Id) == 16 {
		copy(u.ID[:], pbUser.Id)
	}
	u.FirstName = pbUser.FirstName
	u.LastName = pbUser.LastName
	u.BirthDate = pbUser.BirthDate
	u.Active = pbUser.Active
	u.CreatedAt = pbUser.CreatedAt
	u.UpdatedAt = pbUser.UpdatedAt
	u.DeletedAt = pbUser.DeletedAt

	return nil
}
