//go:build darwin

package browser

import (
	"strings"
	"testing"
)

func TestEdgeEstablishedSocketOutputBound(t *testing.T) {
	var out edgeSocketOutput
	if n, err := out.Write([]byte(strings.Repeat("x", edgeSocketLimit))); n != edgeSocketLimit || err != nil {
		t.Fatal("bounded output rejected")
	}
	if n, err := out.Write([]byte("secret overflow")); n != 0 || err != errEdgeProcessUnqualified || out.Len() != edgeSocketLimit {
		t.Fatal("overflow retained")
	}
}
