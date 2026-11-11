//go:build !darwin

package desktopruntime

import (
	"context"
	"errors"
	"io"
)

func RunNextBrowserModeCommand(context.Context, []string, io.Writer, io.Writer) error {
	return errors.New("Next Browser mode management is only available on macOS Next")
}
