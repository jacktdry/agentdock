package process

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func scaledRSS(value string, scale uint64) (*uint64, error) {
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil || scale == 0 || n > math.MaxUint64/scale {
		return nil, fmt.Errorf("invalid RSS %q", value)
	}
	n *= scale
	return &n, nil
}

func parsePS(output string) ([]observedProcess, error) {
	var rows []observedProcess
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 4 {
			return nil, fmt.Errorf("invalid ps column count")
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 {
			return nil, fmt.Errorf("invalid ps PID")
		}
		parent, err := strconv.Atoi(fields[1])
		if err != nil || parent < 0 {
			return nil, fmt.Errorf("invalid ps parent")
		}
		rss, err := scaledRSS(fields[3], 1024)
		if err != nil {
			return nil, err
		}
		rows = append(rows, observedProcess{pid: pid, parent: parent, zombie: strings.HasPrefix(fields[2], "Z"), rss: rss})
	}
	return rows, nil
}

// Linux stat's command can contain spaces, newlines, and parentheses. Only the
// final closing parenthesis delimits it. RSS (field 24) is measured in pages.
func parseProcStat(data string, pageSize uint64) (observedProcess, error) {
	var r observedProcess
	open, close := strings.IndexByte(data, '('), strings.LastIndexByte(data, ')')
	if open < 0 || close <= open {
		return r, fmt.Errorf("invalid proc stat command")
	}
	pid, err := strconv.Atoi(strings.TrimSpace(data[:open]))
	if err != nil || pid <= 0 {
		return r, fmt.Errorf("invalid proc PID")
	}
	fields := strings.Fields(data[close+1:])
	if len(fields) < 22 || len(fields[0]) != 1 {
		return r, fmt.Errorf("invalid proc stat fields")
	}
	parent, err := strconv.Atoi(fields[1])
	if err != nil || parent < 0 {
		return r, fmt.Errorf("invalid proc parent")
	}
	rss, err := scaledRSS(fields[21], pageSize)
	if err != nil {
		return r, err
	}
	return observedProcess{pid: pid, parent: parent, zombie: fields[0] == "Z" || fields[0] == "X", rss: rss}, nil
}
