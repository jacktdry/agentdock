//go:build !darwin

package process

func observeBirthIdentity(int) (BirthIdentity, error) {
	return BirthIdentity{}, ErrIdentityUnavailable
}
