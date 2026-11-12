//go:build darwin

package process

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

var ErrSupervisorDispatchLedgerUnavailable = errors.New("supervisor dispatch ledger unavailable")

const (
	supervisorReceiptLimit    = 64
	supervisorReceiptMaxBytes = 4096
)

type SupervisorDispatchSnapshot struct {
	State         string
	Session       string
	Sequence      uint64
	Core          BirthIdentity
	Worker        BirthIdentity
	RequestSHA256 string
	Outcome       string
	ReplySHA256   string
	Acknowledged  bool
}

// The Supervisor alone opens this read/write state directory. Do NOT expose
// it to Core as an IPC frame field; no operation removes a receipt.
type SupervisorDispatchLedger struct {
	mu     sync.Mutex
	dir    int
	closed bool
}
type SupervisorDispatchTicket struct {
	mu              sync.Mutex
	fd, dir         int
	name            string
	ino, dev        uint64
	closed, settled bool
}

func safeSupervisorDispatchDir(fd int) bool {
	var st unix.Stat_t
	return unix.Fstat(fd, &st) == nil && st.Mode&unix.S_IFMT == unix.S_IFDIR &&
		st.Uid == uint32(os.Geteuid()) && st.Mode&0777 == 0700
}
func safeSupervisorReceipt(fd int) (unix.Stat_t, bool) {
	var st unix.Stat_t
	ok := unix.Fstat(fd, &st) == nil && st.Mode&unix.S_IFMT == unix.S_IFREG &&
		st.Uid == uint32(os.Geteuid()) && st.Mode&0777 == 0600 && st.Nlink == 1
	return st, ok
}
func validDispatchName(name string) bool {
	if len(name) != len("dispatch-")+32+1+16+len(".v1") ||
		!strings.HasPrefix(name, "dispatch-") || !strings.HasSuffix(name, ".v1") {
		return false
	}
	middle := strings.TrimSuffix(strings.TrimPrefix(name, "dispatch-"), ".v1")
	if middle[32] != '-' {
		return false
	}
	for i, r := range middle {
		if i == 32 {
			continue
		}
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
func supervisorReceiptName(frame SupervisorToolCallFrame) (string, error) {
	if frame.Sequence == 0 || len(frame.Session) != 32 {
		return "", ErrSupervisorDispatchLedgerUnavailable
	}
	name := fmt.Sprintf("dispatch-%s-%016x.v1", frame.Session, frame.Sequence)
	if !validDispatchName(name) {
		return "", ErrSupervisorDispatchLedgerUnavailable
	}
	return name, nil
}
func OpenSupervisorDispatchLedger(path string) (*SupervisorDispatchLedger, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, ErrSupervisorDispatchLedgerUnavailable
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrSupervisorDispatchLedgerUnavailable
	}
	if !safeSupervisorDispatchDir(fd) {
		_ = unix.Close(fd)
		return nil, ErrSupervisorDispatchLedgerUnavailable
	}
	return &SupervisorDispatchLedger{dir: fd}, nil
}
func (l *SupervisorDispatchLedger) Close() error {
	if l == nil {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	if err := unix.Close(l.dir); err != nil {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	return nil
}
func countSupervisorReceipts(dir int) (int, error) {
	fd, err := unix.Openat(dir, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return 0, err
	}
	file := os.NewFile(uintptr(fd), "private-receipt-directory")
	defer file.Close()
	entries, err := file.ReadDir(supervisorReceiptLimit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	if len(entries) > supervisorReceiptLimit {
		return 0, ErrSupervisorDispatchLedgerUnavailable
	}
	for _, entry := range entries {
		if !validDispatchName(entry.Name()) {
			return 0, ErrSupervisorDispatchLedgerUnavailable
		}
		var stat unix.Stat_t
		if unix.Fstatat(dir, entry.Name(), &stat, unix.AT_SYMLINK_NOFOLLOW) != nil ||
			stat.Mode&unix.S_IFMT != unix.S_IFREG ||
			stat.Uid != uint32(os.Geteuid()) || stat.Mode&0777 != 0600 || stat.Nlink != 1 ||
			stat.Size < 1 || stat.Size > supervisorReceiptMaxBytes {
			return 0, ErrSupervisorDispatchLedgerUnavailable
		}
	}
	return len(entries), nil
}
func dispatchRequestDigest(frame SupervisorToolCallFrame) string {
	data, err := json.Marshal(frame)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// This is invoked ONLY after the real OS Core peer + authenticated HMAC,
// Supervisor-local allowlist and live Worker identity were checked, but BEFORE
// the sequence is consumed and BEFORE an MCP SDK tool call can run.
func (l *SupervisorDispatchLedger) BeginVerified(frame SupervisorToolCallFrame, core, worker BirthIdentity) (
	*SupervisorDispatchTicket, error) {
	if l == nil || !validLineageBirth(core) || !validLineageBirth(worker) || len(frame.MAC) != 64 {
		return nil, ErrSupervisorDispatchLedgerUnavailable
	}
	name, err := supervisorReceiptName(frame)
	if err != nil {
		return nil, err
	}
	hash := dispatchRequestDigest(frame)
	if hash == "" {
		return nil, ErrSupervisorDispatchLedgerUnavailable
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || !safeSupervisorDispatchDir(l.dir) {
		return nil, ErrSupervisorDispatchLedgerUnavailable
	}
	n, err := countSupervisorReceipts(l.dir)
	if err != nil || n >= supervisorReceiptLimit {
		return nil, ErrSupervisorDispatchLedgerUnavailable
	}
	fd, err := unix.Openat(l.dir, name, unix.O_WRONLY|unix.O_APPEND|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, ErrSupervisorDispatchLedgerUnavailable
	}
	dir, err := unix.Dup(l.dir)
	if err != nil {
		_ = unix.Close(fd)
		return nil, ErrSupervisorDispatchLedgerUnavailable
	}
	st, ok := safeSupervisorReceipt(fd)
	if !ok {
		_ = unix.Close(fd)
		_ = unix.Close(dir)
		return nil, ErrSupervisorDispatchLedgerUnavailable
	}
	ticket := &SupervisorDispatchTicket{fd: fd, dir: dir, name: name, ino: st.Ino, dev: uint64(st.Dev)}
	record := SupervisorDispatchSnapshot{State: "accepted", Session: frame.Session, Sequence: frame.Sequence, Core: core, Worker: worker, RequestSHA256: hash}
	if err := ticket.writeSync(record); err != nil {
		_ = ticket.Close()
		return nil, ErrSupervisorDispatchLedgerUnavailable
	}
	return ticket, nil
}
func (t *SupervisorDispatchTicket) writeSync(record SupervisorDispatchSnapshot) error {
	if t == nil || !safeSupervisorDispatchDir(t.dir) {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	pinned, ok := safeSupervisorReceipt(t.fd)
	var current unix.Stat_t
	if !ok || unix.Fstatat(t.dir, t.name, &current, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		pinned.Ino != t.ino || uint64(pinned.Dev) != t.dev ||
		current.Ino != t.ino || uint64(current.Dev) != t.dev ||
		current.Mode&unix.S_IFMT != unix.S_IFREG || current.Mode&0777 != 0600 ||
		current.Uid != uint32(os.Geteuid()) || current.Nlink != 1 {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	data, err := json.Marshal(record)
	if err != nil || len(data) > supervisorReceiptMaxBytes/2 {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	data = append(data, '\n')
	n, err := unix.Write(t.fd, data)
	if err != nil || n != len(data) || unix.Fsync(t.fd) != nil || unix.Fsync(t.dir) != nil {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	return nil
}

// Outcome means observed SDK completion, NOT that Core received an ACK, or
// that a failed provider tool had no external side effects.
func (t *SupervisorDispatchTicket) Finish(outcome string, reply []byte) error {
	if t == nil || len(reply) > 8192 || (outcome != "succeeded" && outcome != "failed" && outcome != "unknown") {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.settled {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	digest := sha256.Sum256(reply)
	if err := t.writeSync(SupervisorDispatchSnapshot{State: "settled", Outcome: outcome, ReplySHA256: hex.EncodeToString(digest[:])}); err != nil {
		return err
	}
	t.settled = true
	return nil
}
func (t *SupervisorDispatchTicket) Close() error {
	if t == nil {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	a, b := unix.Close(t.fd), unix.Close(t.dir)
	if a != nil || b != nil {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	return nil
}

// Inspect is strictly read-only; accepted-only or truncated receipts never
// authorize a replay, a Core successor, a cleanup, or sticky marker removal.
func InspectSupervisorDispatchReceipt(stateDir string, frame SupervisorToolCallFrame) (SupervisorDispatchSnapshot, error) {
	var zero SupervisorDispatchSnapshot
	name, err := supervisorReceiptName(frame)
	if err != nil {
		return zero, err
	}
	ledger, err := OpenSupervisorDispatchLedger(stateDir)
	if err != nil {
		return zero, err
	}
	defer ledger.Close()
	fd, err := unix.Openat(ledger.dir, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	file := os.NewFile(uintptr(fd), "dispatch-receipt")
	defer file.Close()
	st, ok := safeSupervisorReceipt(fd)
	if !ok || st.Size <= 0 || st.Size > supervisorReceiptMaxBytes {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	data := make([]byte, int(st.Size))
	n, err := io.ReadFull(file, data)
	if err != nil || n != len(data) || data[len(data)-1] != '\n' {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	var pathStat unix.Stat_t
	if unix.Fstatat(ledger.dir, name, &pathStat, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		pathStat.Dev != st.Dev || pathStat.Ino != st.Ino || pathStat.Mode&unix.S_IFMT != unix.S_IFREG ||
		pathStat.Uid != uint32(os.Geteuid()) || pathStat.Mode&0777 != 0600 || pathStat.Nlink != 1 {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	return parseSupervisorDispatchReceipt(data, frame)
}

func parseSupervisorDispatchReceipt(data []byte, frame SupervisorToolCallFrame) (SupervisorDispatchSnapshot, error) {
	var zero SupervisorDispatchSnapshot
	if len(data) == 0 || len(data) > supervisorReceiptMaxBytes || data[len(data)-1] != '\n' {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	lines := bytes.Split(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'})
	if len(lines) < 1 || len(lines) > 3 {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	decode := func(line []byte) (SupervisorDispatchSnapshot, error) {
		var x SupervisorDispatchSnapshot
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&x); err != nil {
			return x, err
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			return x, ErrSupervisorDispatchLedgerUnavailable
		}
		return x, nil
	}
	first, err := decode(lines[0])
	if err != nil || first.State != "accepted" || first.Session != frame.Session ||
		first.Sequence != frame.Sequence || first.RequestSHA256 != dispatchRequestDigest(frame) ||
		!validLineageBirth(first.Core) || !validLineageBirth(first.Worker) || first.Outcome != "" || first.ReplySHA256 != "" || first.Acknowledged {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	if len(lines) == 1 {
		return first, nil
	}
	second, err := decode(lines[1])
	if err != nil || second.State != "settled" ||
		(second.Outcome != "succeeded" && second.Outcome != "failed" && second.Outcome != "unknown") ||
		len(second.ReplySHA256) != 64 || second.Session != "" || second.Sequence != 0 ||
		second.Core.PID != 0 || second.Worker.PID != 0 || second.RequestSHA256 != "" || second.Acknowledged {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	if _, err := hex.DecodeString(second.ReplySHA256); err != nil {
		return zero, ErrSupervisorDispatchLedgerUnavailable
	}
	first.State = "settled"
	first.Outcome = second.Outcome
	first.ReplySHA256 = second.ReplySHA256
	if len(lines) == 3 {
		ack, err := decode(lines[2])
		if err != nil || ack.State != "acknowledged" || ack.ReplySHA256 != second.ReplySHA256 ||
			ack.Session != "" || ack.Sequence != 0 || ack.Core.PID != 0 || ack.Worker.PID != 0 ||
			ack.Outcome != "" || ack.RequestSHA256 != "" || ack.Acknowledged {
			return zero, ErrSupervisorDispatchLedgerUnavailable
		}
		first.Acknowledged = true
	}
	return first, nil
}
