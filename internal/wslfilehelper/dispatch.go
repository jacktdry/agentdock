//go:build linux

package wslfilehelper

func Dispatch(req *Request) (*Response, error) {
	return dispatchWithOps(req, defaultTransactionOps())
}

func dispatchWithOps(req *Request, ops transactionOps) (*Response, error) {
	if req == nil {
		return nil, fail("INVALID_ARGUMENT", "WSL file helper request is required", nil)
	}
	includeAncestors := req.Action != "read" && req.Action != "list_dir" && req.Action != "search_text"
	for _, target := range append([]string{req.Path, req.NewPath}, changePaths(req.Changes)...) {
		if target != "" && controlPathBlocked(req.ProtectedRoots, target, includeAncestors) {
			return nil, fail("PROTECTED_CONTROL_PATH", "AgentDock control-plane paths are not accessible through WSL file tools", nil)
		}
	}
	if req.Workdir != "" && controlPathBlocked(req.ProtectedRoots, req.Workdir, false) {
		return nil, fail("PROTECTED_CONTROL_PATH", "AgentDock control-plane workdir is protected", nil)
	}
	switch req.Action {
	case "read":
		return readText(req.Path, req.RejectSymlink, req.AllowMissing)
	case "list_dir":
		return listDirectory(req)
	case "search_text":
		return searchText(req)
	case "write_atomic":
		return atomicWrite(req)
	case "delete":
		return deleteFile(req)
	case "move":
		return moveFile(req)
	case "patch_transaction":
		return patchTransaction(req, ops)
	case "recover_patch_transactions":
		return recoverPatchTransactions(req, ops)
	default:
		return nil, fail("INVALID_ACTION", "unsupported WSL file helper action", map[string]any{"action": req.Action})
	}
}

func changePaths(changes []ChangeRequest) []string {
	paths := make([]string, 0, len(changes))
	for _, change := range changes {
		paths = append(paths, change.Path)
	}
	return paths
}
