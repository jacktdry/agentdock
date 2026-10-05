//go:build darwin && cgo

package desktopruntime

/*
#include <libproc.h>
#include <sys/proc_info.h>
static int port_process(int pid, char *path, struct proc_bsdinfo *info) {
    if (proc_pidpath(pid, path, PROC_PIDPATHINFO_MAXSIZE) <= 0) return 0;
    return proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, info, sizeof(*info)) == sizeof(*info);
}
*/
import "C"

import "fmt"

func platformPortProcess(pid int) (string, string, error) {
	if pid <= 0 {
		return "", "", ErrPortPreflight
	}
	var path [C.PROC_PIDPATHINFO_MAXSIZE]C.char
	var info C.struct_proc_bsdinfo
	if C.port_process(C.int(pid), &path[0], &info) == 0 {
		return "", "", ErrPortPreflight
	}
	return C.GoString(&path[0]), fmt.Sprintf("%d:%d", uint64(info.pbi_start_tvsec), uint64(info.pbi_start_tvusec)), nil
}
