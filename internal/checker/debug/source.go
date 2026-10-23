package debug

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
)

type sourceFinding struct {
	Line int
	Name string
}

var jsRegexPrefix = regexp.MustCompile(`(?:^|[^\w$])(?:return|typeof|case|delete|void|in|instanceof|new|throw|yield|await)$`)

// maskJS blanks comments, string, template and regex literal text, keeping newlines and code inside ${}.
func maskJS(source []byte) []byte {
	masked := bytes.Clone(source)
	blank := func(i int) {
		if masked[i] != '\n' {
			masked[i] = ' '
		}
	}
	// Each entry is the brace depth at which a template ${} expression returns to its literal.
	var templates []int
	depth := 0
	lastSignificant := func(i int) (byte, int) {
		for j := i - 1; j >= 0; j-- {
			if c := masked[j]; c != ' ' && c != '\t' && c != '\n' && c != '\r' {
				return c, j
			}
		}
		return 0, -1
	}
	regexAllowed := func(i int) bool {
		c, j := lastSignificant(i)
		if j < 0 || strings.IndexByte("(,=:[!&|?{};+-*%<>~^", c) >= 0 {
			return true
		}
		return jsRegexPrefix.Match(masked[:j+1])
	}
	for i := 0; i < len(masked); i++ {
		c := masked[i]
		switch {
		case c == '/' && i+1 < len(masked) && masked[i+1] == '/':
			for ; i < len(masked) && masked[i] != '\n'; i++ {
				blank(i)
			}
		case c == '/' && i+1 < len(masked) && masked[i+1] == '*':
			blank(i)
			blank(i + 1)
			for i += 2; i < len(masked) && !(masked[i] == '*' && i+1 < len(masked) && masked[i+1] == '/'); i++ {
				blank(i)
			}
			if i < len(masked) {
				blank(i)
				blank(i + 1)
				i++
			}
		case c == '\'' || c == '"':
			for i++; i < len(masked) && masked[i] != c && masked[i] != '\n'; i++ {
				if masked[i] == '\\' && i+1 < len(masked) {
					blank(i)
					i++
				}
				blank(i)
			}
		case c == '/' && regexAllowed(i):
			class := false
			for i++; i < len(masked) && masked[i] != '\n' && (class || masked[i] != '/'); i++ {
				if masked[i] == '\\' && i+1 < len(masked) {
					blank(i)
					i++
				} else if masked[i] == '[' {
					class = true
				} else if masked[i] == ']' {
					class = false
				}
				blank(i)
			}
		case c == '`' || c == '}' && len(templates) > 0 && templates[len(templates)-1] == depth:
			if c == '}' {
				templates = templates[:len(templates)-1]
			}
			for i++; i < len(masked) && masked[i] != '`'; i++ {
				if masked[i] == '\\' && i+1 < len(masked) {
					blank(i)
					i++
				} else if masked[i] == '$' && i+1 < len(masked) && masked[i+1] == '{' {
					templates = append(templates, depth)
					i++
					break
				}
				blank(i)
			}
		case c == '{':
			depth++
		case c == '}':
			depth--
		}
	}
	return masked
}

var twigVerbatim = regexp.MustCompile(`^\{%-?~?\s*(verbatim|raw)\s*-?~?%\}`)

var twigVerbatimEnd = map[string]*regexp.Regexp{
	"verbatim": regexp.MustCompile(`\{%-?~?\s*endverbatim\s*-?~?%\}`),
	"raw":      regexp.MustCompile(`\{%-?~?\s*endraw\s*-?~?%\}`),
}

// maskTwig keeps only the code inside {{ }} and {% %} tags, with string literals blanked.
func maskTwig(source []byte) []byte {
	masked := bytes.Repeat([]byte{' '}, len(source))
	for i, c := range source {
		if c == '\n' {
			masked[i] = '\n'
		}
	}
	for i := 0; i+1 < len(source); i++ {
		if source[i] != '{' {
			continue
		}
		var closing string
		switch source[i+1] {
		case '#':
			closing = "#}"
		case '{':
			closing = "}}"
		case '%':
			closing = "%}"
		default:
			continue
		}
		if match := twigVerbatim.FindSubmatch(source[i:]); match != nil {
			end := twigVerbatimEnd[string(match[1])].FindIndex(source[i:])
			if end == nil {
				return masked
			}
			i += end[1] - 1
			continue
		}
		start := i + 2
		quote := byte(0)
		for i = start; i+1 < len(source); i++ {
			c := source[i]
			if quote != 0 {
				if c == '\\' {
					i++
				} else if c == quote {
					quote = 0
				}
				continue
			}
			if closing != "#}" && (c == '\'' || c == '"') {
				quote = c
				continue
			}
			if string(source[i:i+2]) == closing {
				break
			}
			if closing != "#}" {
				masked[i] = c
			}
		}
		i++
	}
	return masked
}

// calls finds name( in masked source, where name is a dot path not preceded by an identifier or dot.
func calls(masked []byte, names []string) []sourceFinding {
	var result []sourceFinding
	for _, name := range names {
		parts := strings.Split(name, ".")
		for i, part := range parts {
			parts[i] = regexp.QuoteMeta(part)
		}
		pattern := regexp.MustCompile(`(?:^|[^\w$.])(` + strings.Join(parts, `\s*\.\s*`) + `)\s*\(`)
		for _, match := range pattern.FindAllSubmatchIndex(masked, -1) {
			result = append(result, sourceFinding{Line: bytes.Count(masked[:match[2]], []byte("\n")) + 1, Name: name})
		}
	}
	slices.SortStableFunc(result, func(a, b sourceFinding) int { return a.Line - b.Line })
	return result
}

var debuggerStatement = regexp.MustCompile(`(?:^|[^\w$.])(debugger)\b\s*([^\s:]|$)`)

func debuggers(masked []byte) []int {
	var lines []int
	for _, match := range debuggerStatement.FindAllSubmatchIndex(masked, -1) {
		lines = append(lines, bytes.Count(masked[:match[2]], []byte("\n"))+1)
	}
	return lines
}
