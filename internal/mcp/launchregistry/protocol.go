// Package launchregistry contains an isolated, diagnostic-only protocol for
// MCP servers explicitly launched by a Next-owned Codex ACP stdio wrapper.
// Registering an OS child is not Thread ownership or permission to terminate.
package launchregistry

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"regexp"
	"strings"

	"github.com/uvwt/agentdock/internal/process"
)

var ErrUnavailable = errors.New("MCP launch registration unavailable")
var serverPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,80}$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

const maxFrame = 2048

type frame struct {
	Stage        string                 `json:"stage"`
	Server       string                 `json:"server,omitempty"`
	ConfigDigest string                 `json:"config_digest,omitempty"`
	Nonce        string                 `json:"nonce,omitempty"`
	Child        *process.BirthIdentity `json:"child,omitempty"`
}

// Observation is a short-lived, read-only kernel correspondence. It conveys
// neither native Codex Thread attribution nor cleanup authority.
type Observation struct {
	Server       string
	ConfigDigest string
	AppServer    process.BirthIdentity
	Wrapper      process.BirthIdentity
	Child        process.BirthIdentity
}

func validServer(id string) bool { return serverPattern.MatchString(id) }
func validDigest(d string) bool  { return digestPattern.MatchString(d) }

func readFrame(reader *bufio.Reader) (frame, error) {
	line, err := reader.ReadSlice('\n')
	if err != nil || len(line) < 2 || len(line) > maxFrame {
		return frame{}, ErrUnavailable
	}
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	var data frame
	if rejectsDuplicateKeys(line) != nil || dec.Decode(&data) != nil {
		return frame{}, ErrUnavailable
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return frame{}, ErrUnavailable
	}
	return data, nil
}
func writeFrame(conn net.Conn, f frame) error {
	data, err := json.Marshal(f)
	if err != nil || len(data)+1 > maxFrame {
		return ErrUnavailable
	}
	data = append(data, '\n')
	for len(data) > 0 {
		n, err := conn.Write(data)
		if err != nil || n == 0 {
			return ErrUnavailable
		}
		data = data[n:]
	}
	return nil
}
func newNonce() (string, error) {
	var b [32]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}
func validNonce(n string) bool { return len(n) == 64 && digestPattern.MatchString(n) }
func strippedChildEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, v := range env {
		if !strings.HasPrefix(v, "AGENTDOCK_NEXT_MCP_LAUNCH_") {
			out = append(out, v)
		}
	}
	return out
}

// JSON permits repeated object keys by default, but this protocol must not
// let multiple interpretations of a server, nonce, or child identity through.
func rejectsDuplicateKeys(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	var walk func() error
	walk = func() error {
		tok, err := d.Token()
		if err != nil {
			return ErrUnavailable
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]bool)
			for d.More() {
				key, err := d.Token()
				name, ok := key.(string)
				if err != nil || !ok || seen[name] {
					return ErrUnavailable
				}
				seen[name] = true
				if walk() != nil {
					return ErrUnavailable
				}
			}
			if tok, err = d.Token(); err != nil || tok != json.Delim('}') {
				return ErrUnavailable
			}
		case '[':
			for d.More() {
				if walk() != nil {
					return ErrUnavailable
				}
			}
			if tok, err = d.Token(); err != nil || tok != json.Delim(']') {
				return ErrUnavailable
			}
		default:
			return ErrUnavailable
		}
		return nil
	}
	if err := walk(); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return ErrUnavailable
	}
	return nil
}
