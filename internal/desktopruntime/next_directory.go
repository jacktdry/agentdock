package desktopruntime

import "errors"

type NextDirectoryKind string

const (
	NextDirectoryLogs          NextDirectoryKind = "logs"
	NextDirectoryConfiguration NextDirectoryKind = "configuration"
)

var ErrDirectoryRequiresNative = errors.New("directory_requires_native")
var errDirectoryUnavailable = errors.New("directory_unavailable")
