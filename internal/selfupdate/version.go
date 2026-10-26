package selfupdate

import (
	"fmt"
	"regexp"
	"strings"
)

var releaseVersion = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)
var digits = regexp.MustCompile(`^[0-9]+$`)

func Version(value string) (string, error) {
	value = "v" + strings.TrimPrefix(value, "v")
	match := releaseVersion.FindStringSubmatch(value)
	if match == nil || len(value) > 128 {
		return "", fmt.Errorf("version must be MAJOR.MINOR.PATCH or MAJOR.MINOR.PATCH-prerelease")
	}
	for _, part := range strings.Split(match[4], ".") {
		if digits.MatchString(part) && len(part) > 1 && part[0] == '0' {
			return "", fmt.Errorf("numeric prerelease identifiers cannot start with zero")
		}
	}
	return value, nil
}

func Compare(a, b string) int {
	x, y := releaseVersion.FindStringSubmatch(a), releaseVersion.FindStringSubmatch(b)
	for i := 1; i < 4; i++ {
		if result := numeric(x[i], y[i]); result != 0 {
			return result
		}
	}
	if x[4] == y[4] {
		return 0
	}
	if x[4] == "" {
		return 1
	}
	if y[4] == "" {
		return -1
	}
	left, right := strings.Split(x[4], "."), strings.Split(y[4], ".")
	for i := 0; i < min(len(left), len(right)); i++ {
		if left[i] == right[i] {
			continue
		}
		a, b := digits.MatchString(left[i]), digits.MatchString(right[i])
		if a && b {
			return numeric(left[i], right[i])
		}
		if a {
			return -1
		}
		if b {
			return 1
		}
		return strings.Compare(left[i], right[i])
	}
	return numeric(fmt.Sprint(len(left)), fmt.Sprint(len(right)))
}

func numeric(a, b string) int {
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return strings.Compare(a, b)
}
