package user

import (
	"github.com/kopia/kopia/repo/manifest"
)

// Profile describes information about a single user.
type Profile struct {
	ManifestID manifest.ID `json:"-"`

	Username            string `json:"username"`
	PasswordHashVersion int    `json:"passwordHashVersion"` // indicates how password is hashed
	PasswordHash        []byte `json:"passwordHash"`
}

// SetPassword changes the password for a user profile.
func (p *Profile) SetPassword(password string) error {
	return p.setPasswordV1(password)
}

// IsValidPassword determines whether the password is valid for a given user.
func (p *Profile) IsValidPassword(password string) bool {
	if p == nil {
		// if the user is invalid, return false but use the same amount of time as when we
		// compare against valid user to avoid revealing whether the user account exists.
		isValidPasswordV1(password, dummyV1HashThatNeverMatchesAnyPassword)

		return false
	}

	switch p.PasswordHashVersion {
	case hashVersion1:
		return isValidPasswordV1(password, p.PasswordHash)

	default:
		return false
	}
}
