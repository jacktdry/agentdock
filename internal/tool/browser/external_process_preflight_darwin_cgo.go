//go:build darwin && cgo

package browser

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <libproc.h>

static int edge_live_signature(int pid, const char *requirementText) {
    int valid = 0;
    CFNumberRef number = NULL;
    CFDictionaryRef attributes = NULL;
    CFStringRef text = NULL;
    SecCodeRef code = NULL;
    SecRequirementRef requirement = NULL;
    if (pid <= 0) return 0;
    number = CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &pid);
    if (!number) goto cleanup;
    const void *keys[] = { kSecGuestAttributePid };
    const void *values[] = { number };
    attributes = CFDictionaryCreate(kCFAllocatorDefault, keys, values, 1,
        &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    if (!attributes) goto cleanup;
    if (SecCodeCopyGuestWithAttributes(NULL, attributes, kSecCSDefaultFlags, &code) != errSecSuccess || !code) goto cleanup;
    text = CFStringCreateWithCString(kCFAllocatorDefault, requirementText, kCFStringEncodingUTF8);
    if (!text) goto cleanup;
    if (SecRequirementCreateWithString(text, kSecCSDefaultFlags, &requirement) != errSecSuccess || !requirement) goto cleanup;
    valid = SecCodeCheckValidity(code, kSecCSDefaultFlags, requirement) == errSecSuccess;
cleanup:
    if (requirement) CFRelease(requirement);
    if (text) CFRelease(text);
    if (code) CFRelease(code);
    if (attributes) CFRelease(attributes);
    if (number) CFRelease(number);
    return valid;
}
*/
import "C"

import (
	"context"
	"strings"
	"time"
	"unsafe"
)

// Kernel-reported running executable; unlike lsof's txt catalog this is not
// proportional to Edge's loaded libraries. Signature and epoch checks remain
// independent requirements before/after this bounded native observation.
func macEdgeExecutablePath(ctx context.Context, pid int) (string, error) {
	if ctx == nil || ctx.Err() != nil || pid <= 0 || int64(pid) > 2147483647 {
		return "", errEdgeProcessUnqualified
	}
	var buf [C.PROC_PIDPATHINFO_MAXSIZE]C.char
	n := C.proc_pidpath(C.int(pid), unsafe.Pointer(&buf[0]), C.uint32_t(len(buf)))
	if n <= 0 || int(n) >= len(buf) || ctx.Err() != nil {
		return "", errEdgeProcessUnqualified
	}
	path := C.GoStringN(&buf[0], C.int(n))
	if strings.ContainsRune(path, 0) {
		return "", errEdgeProcessUnqualified
	}
	return path, nil
}

// Security.framework calls cannot be canceled. Keep the slot until native
// verification actually returns, even after the caller times out. A stuck call
// thus retains at most one worker; subsequent callers fail closed immediately.
type edgeLiveSignatureVerifier struct {
	slot   chan struct{}
	verify func(int) bool
}

var macEdgeLiveSignature = edgeLiveSignatureVerifier{
	slot: make(chan struct{}, 1),
	verify: func(pid int) bool {
		text := C.CString(edgeSignerRequirement)
		defer C.free(unsafe.Pointer(text))
		return C.edge_live_signature(C.int(pid), text) == 1
	},
}

func (v *edgeLiveSignatureVerifier) check(ctx context.Context, pid int) error {
	if ctx == nil || ctx.Err() != nil || pid <= 0 || int64(pid) > 2147483647 || v.verify == nil || cap(v.slot) != 1 {
		return errEdgeProcessUnqualified
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	select {
	case v.slot <- struct{}{}:
	default:
		return errEdgeProcessUnqualified
	}
	result := make(chan bool, 1)
	go func() {
		verified := false
		defer func() {
			// Native bindings normally return a status; any unexpected Go
			// panic in the verifier must also release the slot and reject.
			_ = recover()
			result <- verified
			<-v.slot
		}()
		if ctx.Err() == nil {
			verified = v.verify(pid)
		}
	}()
	select {
	case valid := <-result:
		if valid && ctx.Err() == nil {
			return nil
		}
	case <-ctx.Done():
	}
	return errEdgeProcessUnqualified
}

func (macEdgeProcessProbe) liveSignature(ctx context.Context, pid int) error {
	return macEdgeLiveSignature.check(ctx, pid)
}
