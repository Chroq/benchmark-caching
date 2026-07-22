package model

type ID [16]byte

// UserData represents the domain cache entry entity.
type UserData struct {
	FirstName string
	LastName  string
	BirthDate int64
	CreatedAt int64
	UpdatedAt int64
	DeletedAt int64
	ID        ID
	Active    bool
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
