//go:build !darwin

package process

func observeGroupBirths(int) ([]BirthIdentity, error) {
	return nil, ErrGroupObservationUnavailable
}
