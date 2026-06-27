package handler

import (
	"errors"
	"strconv"
	"strings"
)

// UpstreamLineFields holds the parsed columns of one CSV-lite row used by the
// upstream batch importer. Layout: `addr[,weight][,backup|main][,remark]`.
//
// - Leading / trailing whitespace per field is trimmed.
// - Missing trailing fields receive defaults: weight=1, backup=false, remark="".
// - A blank middle field also gets its default (e.g. "addr,,," means "addr only").
// - The remark field MAY be wrapped in double-quotes when it contains commas;
//   the surrounding quotes are stripped, internal commas preserved.
type UpstreamLineFields struct {
	Addr     string
	Weight   int
	IsBackup bool
	Remark   string
}

var errEmptyLine = errors.New("空行")

// ParseUpstreamLine parses a single CSV-lite row. Returns a descriptive error
// when the addr field is missing/malformed; weight/backup parse errors map to
// "权重不合法" / "备用字段不合法 (期望 backup|main)".
func ParseUpstreamLine(line string) (*UpstreamLineFields, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return nil, errEmptyLine
	}

	fields := splitCSVLite(trimmed)
	if len(fields) == 0 || fields[0] == "" {
		return nil, errors.New("缺少 addr 字段")
	}

	out := &UpstreamLineFields{
		Addr:   fields[0],
		Weight: 1,
	}

	if len(fields) >= 2 && fields[1] != "" {
		v, err := strconv.Atoi(fields[1])
		if err != nil || v <= 0 || v > 100 {
			return nil, errors.New("权重不合法 (期望 1-100 的整数)")
		}
		out.Weight = v
	}

	if len(fields) >= 3 && fields[2] != "" {
		switch strings.ToLower(fields[2]) {
		case "backup", "b", "true", "1":
			out.IsBackup = true
		case "main", "m", "false", "0":
			out.IsBackup = false
		default:
			return nil, errors.New("备用字段不合法 (期望 backup|main)")
		}
	}

	if len(fields) >= 4 {
		out.Remark = fields[3]
	}

	return out, nil
}

// splitCSVLite splits on commas, honoring a single pair of double-quotes around
// any single field. Quotes are stripped from the field value. Implemented as a
// 4-state machine so that leading whitespace before a `"` does not break quote
// detection, and so commas inside quotes are preserved.
//
// This is a minimal CSV-lite — no escape sequences, no embedded newlines.
func splitCSVLite(s string) []string {
	const (
		stateNewField   = iota // expecting field start; eating leading whitespace
		stateInField           // unquoted field content
		stateInQuotes          // inside "...", commas are literal
		stateAfterQuote        // just closed "...", eating trailing whitespace until ','
	)

	var (
		out   []string
		field strings.Builder
		state = stateNewField
	)

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch state {
		case stateNewField:
			switch c {
			case ' ', '\t':
				// skip leading whitespace
			case '"':
				state = stateInQuotes
			case ',':
				out = append(out, "")
				// state stays stateNewField
			default:
				field.WriteByte(c)
				state = stateInField
			}
		case stateInField:
			if c == ',' {
				out = append(out, strings.TrimRight(field.String(), " \t"))
				field.Reset()
				state = stateNewField
				continue
			}
			field.WriteByte(c)
		case stateInQuotes:
			if c == '"' {
				state = stateAfterQuote
				continue
			}
			field.WriteByte(c)
		case stateAfterQuote:
			switch c {
			case ',':
				out = append(out, field.String())
				field.Reset()
				state = stateNewField
			case ' ', '\t':
				// eat trailing whitespace
			default:
				// Garbage after closing quote — treat as part of field.
				field.WriteByte(c)
			}
		}
	}

	// Flush whatever's in the buffer for the final field.
	switch state {
	case stateInField, stateAfterQuote, stateInQuotes:
		out = append(out, strings.TrimRight(field.String(), " \t"))
	case stateNewField:
		out = append(out, "")
	}
	return out
}
