package model

type ID [16]byte

// UserData represents the domain cache entry entity.
type UserData struct {
	ID        ID     // 16-byte raw UUID/ULID
	FirstName string // Dynamic length string
	LastName  string // Dynamic length string
	BirthDate int64  // Unix Timestamp
	Active    bool   // Boolean
	CreatedAt int64  // Unix Timestamp
	UpdatedAt int64  // Unix Timestamp
	DeletedAt int64  // Unix Timestamp
}

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
