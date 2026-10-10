//go:build !darwin

package process

func uidProcessBirths(uint32) ([]BirthIdentity, error) {
	return nil, ErrDescendantObservationUnavailable
}
