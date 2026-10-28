package inputsource

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"unicode/utf8"
)

type Position struct {
	Line, Column int
}

type Error struct {
	File, Path, Message string
	Position            Position
	Previous            *Position
	Err                 error
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) Error() string {
	location := e.File
	if e.Position.Line > 0 {
		if location != "" {
			location += fmt.Sprintf(":%d", e.Position.Line)
			if e.Position.Column > 0 {
				location += fmt.Sprintf(":%d", e.Position.Column)
			}
		} else {
			location = fmt.Sprintf("line %d", e.Position.Line)
			if e.Position.Column > 0 {
				location += fmt.Sprintf(", column %d", e.Position.Column)
			}
		}
	}
	if e.Path != "" {
		location += " (" + e.Path + ")"
	}
	text := e.Message
	if location != "" {
		text = location + ": " + text
	}
	if e.Previous != nil {
		text += fmt.Sprintf("; first defined at line %d, column %d", e.Previous.Line, e.Previous.Column)
	}
	return text
}

func WithFile(err error, name string) error {
	var source *Error
	if errors.As(err, &source) {
		copy := *source
		copy.File = name
		return &copy
	}
	return fmt.Errorf("%s: %w", name, err)
}

func At(data []byte, offset int64) Position {
	offset = max(0, min(offset, int64(len(data))))
	before := data[:offset]
	line, start := 1, 0
	for i, char := range before {
		if char == '\r' {
			line, start = line+1, i+1
		} else if char == '\n' {
			if i == 0 || before[i-1] != '\r' {
				line++
			}
			start = i + 1
		}
	}
	return Position{Line: line, Column: utf8.RuneCount(before[start:]) + 1}
}

func TokenStart(data []byte, offset int64) int64 {
	for offset < int64(len(data)) && (data[offset] == ' ' || data[offset] == '\t' || data[offset] == '\r' || data[offset] == '\n' || data[offset] == ',' || data[offset] == ':') {
		offset++
	}
	return offset
}

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func Member(parent, key string) string {
	if identifier.MatchString(key) {
		return parent + "." + key
	}
	quoted, _ := json.Marshal(key)
	return parent + "[" + string(quoted) + "]"
}
