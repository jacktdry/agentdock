//go:build darwin

package browser

import (
	"context"
	"os"
	"syscall"
)

func (macEdgeProcessProbe) profileMetadata(ctx context.Context, dir string) (edgeProfileMetadata, error) {
	return readEdgeProfileMetadata(ctx, dir, uint32(os.Getuid()), func(path string) (edgeProfileComponent, error) {
		info, err := os.Lstat(path)
		if err != nil {
			return edgeProfileComponent{}, errEdgeProcessUnqualified
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return edgeProfileComponent{}, errEdgeProcessUnqualified
		}
		return edgeProfileComponent{device: uint64(stat.Dev), inode: stat.Ino, uid: stat.Uid, mode: info.Mode()}, nil
	})
}
