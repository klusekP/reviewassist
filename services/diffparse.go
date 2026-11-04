package services

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// DiffLines describes which lines of a unified diff can carry an inline comment.
type DiffLines struct {
	Added    map[int]bool // new_line numbers of added lines
	Context  map[int]int  // new_line -> old_line for unchanged lines visible in the diff
	Numbered string       // diff annotated with new-file line numbers for the model
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// ParseDiff parses a GitLab unified diff (without file headers) and annotates it with line numbers.
func ParseDiff(diff string) *DiffLines {
	res := &DiffLines{Added: map[int]bool{}, Context: map[int]int{}}
	var b strings.Builder
	oldLine, newLine := 0, 0

	for _, line := range strings.Split(diff, "\n") {
		if m := hunkHeader.FindStringSubmatch(line); m != nil {
			oldLine, _ = strconv.Atoi(m[1])
			newLine, _ = strconv.Atoi(m[2])
			b.WriteString(line + "\n")
			continue
		}
		if line == "" || strings.HasPrefix(line, `\ No newline`) {
			continue
		}
		switch line[0] {
		case '+':
			res.Added[newLine] = true
			fmt.Fprintf(&b, "%5d + %s\n", newLine, line[1:])
			newLine++
		case '-':
			fmt.Fprintf(&b, "      - %s\n", line[1:])
			oldLine++
		default:
			res.Context[newLine] = oldLine
			fmt.Fprintf(&b, "%5d   %s\n", newLine, strings.TrimPrefix(line, " "))
			oldLine++
			newLine++
		}
	}
	res.Numbered = b.String()
	return res
}

// ResolveLine finds the best commentable line for the line reported by the model.
// Only lines added/modified by the MR are accepted (unchanged context lines are out of scope).
// Returns new_line, old_line (always 0 for added lines) and ok=false when the line is not a changed line.
func (d *DiffLines) ResolveLine(line int) (newLine, oldLine int, ok bool) {
	if line <= 0 {
		return 0, 0, false
	}
	if d.Added[line] {
		return line, 0, true
	}
	// Models are often off by a line or two - snap to the nearest added line.
	for delta := 1; delta <= 3; delta++ {
		if d.Added[line+delta] {
			return line + delta, 0, true
		}
		if d.Added[line-delta] {
			return line - delta, 0, true
		}
	}
	return 0, 0, false
}

// issueLine converts the loosely typed "line" from the model into an int.
func issueLine(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		// handle "12", "12-15", "L12"
		t = strings.TrimLeft(strings.TrimSpace(t), "Ll")
		if i := strings.IndexAny(t, "-, "); i > 0 {
			t = t[:i]
		}
		n, _ := strconv.Atoi(t)
		return n
	}
	return 0
}
