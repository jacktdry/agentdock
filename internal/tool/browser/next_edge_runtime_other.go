//go:build !darwin

package browser

func nativeEdgePlatformDependencies() (nativeEdgeDependencies, error) {
	return nativeEdgeDependencies{}, externalPeerVerificationError()
}
