package process

import "errors"

var ErrIndependentExitObserverUnavailable = errors.New("independent exit observer unavailable")

// IndependentExitRequest contains exact Core + worker kernel birth identities
// for a source-only independent read-only observer. It grants NO authority
// to kill, restart, unlink Core's sticky marker or own Chrome descendants.
type IndependentExitRequest struct {
	Core        BirthIdentity `json:"core"`
	Worker      BirthIdentity `json:"worker"`
	StateDir    string        `json:"state_dir"`
	ReceiptName string        `json:"receipt_name"`
}
