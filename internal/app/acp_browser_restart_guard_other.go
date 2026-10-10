//go:build !darwin

package app

import "errors"

type sharedBrowserRestartGuard struct{}

func acquireSharedBrowserRestartGuard(string) (*sharedBrowserRestartGuard, error) {
	return nil, errors.New("Next shared browser restart guard unavailable")
}
func (*sharedBrowserRestartGuard) releaseAfterVerifiedClose() error {
	return errors.New("Next shared browser restart guard unavailable")
}
