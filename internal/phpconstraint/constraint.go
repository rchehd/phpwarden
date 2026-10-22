package phpconstraint

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const maxPart = uint64(1<<32 - 1)

type version [3]uint64

type interval struct {
	low  version
	high *version
}

type Set struct {
	ranges []interval
}

var atom = regexp.MustCompile(`^(>=|<=|!=|<>|==|=|>|<|\^|~)?\s*(v?[0-9xX*]+(?:\.[0-9xX*]+){0,2})`)
var alternatives = regexp.MustCompile(`\|\|?`)
var hyphen = regexp.MustCompile(`^\s+-\s+`)

func Any() Set { return Set{ranges: []interval{{}}} }

func Parse(text string) (Set, error) {
	if len(text) > 4096 || strings.TrimSpace(text) == "" {
		return Set{}, fmt.Errorf("empty or oversized PHP constraint")
	}
	result := Set{}
	for _, branch := range alternatives.Split(text, -1) {
		branch = strings.TrimSpace(branch)
		if branch == "" {
			return Set{}, fmt.Errorf("empty PHP constraint alternative")
		}
		conjunction := Any()
		for branch != "" {
			match := atom.FindStringSubmatch(branch)
			if match == nil {
				return Set{}, fmt.Errorf("unsupported PHP constraint syntax near %q", branch)
			}
			term, err := parseAtom(match[1], match[2])
			if err != nil {
				return Set{}, err
			}
			branch = branch[len(match[0]):]
			if prefix := hyphen.FindString(branch); prefix != "" {
				if match[1] != "" || strings.ContainsAny(match[2], "*xX") {
					return Set{}, fmt.Errorf("invalid hyphenated PHP constraint")
				}
				branch = branch[len(prefix):]
				right := atom.FindStringSubmatch(branch)
				if right == nil || right[1] != "" || strings.ContainsAny(right[2], "*xX") {
					return Set{}, fmt.Errorf("invalid hyphenated PHP constraint endpoint")
				}
				low, _, _, _ := parseVersion(match[2])
				high, count, _, err := parseVersion(right[2])
				if err != nil {
					return Set{}, err
				}
				term = Set{ranges: []interval{{low: low, high: next(high, count-1)}}}
				branch = branch[len(right[0]):]
			}
			conjunction = conjunction.Intersect(term)
			if branch != "" && !strings.ContainsAny(branch[:1], " \t\r\n,") {
				return Set{}, fmt.Errorf("unsupported PHP constraint suffix %q", branch)
			}
			branch = strings.TrimSpace(branch)
			if strings.HasPrefix(branch, ",") {
				branch = strings.TrimSpace(branch[1:])
				if branch == "" || strings.HasPrefix(branch, ",") {
					return Set{}, fmt.Errorf("invalid PHP constraint conjunction")
				}
			}
		}
		result.ranges = append(result.ranges, conjunction.ranges...)
	}
	return normalize(result), nil
}

func parseAtom(operator, text string) (Set, error) {
	value, count, wildcard, err := parseVersion(text)
	if err != nil {
		return Set{}, err
	}
	var high *version
	if wildcard {
		if count > 0 {
			high = next(value, count-1)
		}
		if operator != "" && operator != "=" && operator != "==" {
			return Set{}, fmt.Errorf("unsupported wildcard operator %q", operator)
		}
	} else {
		high = next(value, 2)
		switch operator {
		case "^":
			index := 0
			for index < count-1 && value[index] == 0 {
				index++
			}
			high = next(value, index)
		case "~":
			index := 0
			if count == 3 {
				index = 1
			}
			high = next(value, index)
		case ">=":
			return Set{ranges: []interval{{low: value}}}, nil
		case ">":
			if high == nil {
				return Set{}, nil
			}
			return Set{ranges: []interval{{low: *high}}}, nil
		case "<":
			return normalize(Set{ranges: []interval{{high: &value}}}), nil
		case "<=":
			return Set{ranges: []interval{{high: high}}}, nil
		}
	}
	result := normalize(Set{ranges: []interval{{low: value, high: high}}})
	if operator == "!=" || operator == "<>" {
		result = result.Complement()
	}
	return result, nil
}

func parseVersion(text string) (version, int, bool, error) {
	var value version
	parts := strings.Split(strings.TrimPrefix(text, "v"), ".")
	if len(parts) > 3 || len(parts) == 0 {
		return value, 0, false, fmt.Errorf("expected a stable numeric PHP version")
	}
	count, wildcard := 0, false
	for i, part := range parts {
		if part == "*" || strings.EqualFold(part, "x") {
			wildcard = true
			continue
		}
		if wildcard || part == "" {
			return value, 0, false, fmt.Errorf("invalid PHP version %q", text)
		}
		number, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return value, 0, false, fmt.Errorf("invalid PHP version %q", text)
		}
		value[i] = number
		count++
	}
	return value, count, wildcard, nil
}

func StableVersion(text string) (string, error) {
	value, _, wildcard, err := parseVersion(text)
	if err != nil || wildcard {
		return "", fmt.Errorf("expected a stable numeric PHP version, got %q", text)
	}
	return fmt.Sprintf("%d.%d.%d", value[0], value[1], value[2]), nil
}

func (set Set) Allows(text string) (bool, error) {
	value, _, wildcard, err := parseVersion(text)
	if err != nil || wildcard {
		return false, fmt.Errorf("cannot evaluate non-stable PHP version %q", text)
	}
	for _, interval := range set.ranges {
		if compare(value, interval.low) >= 0 && (interval.high == nil || compare(value, *interval.high) < 0) {
			return true, nil
		}
	}
	return false, nil
}

func (set Set) Intersect(other Set) Set {
	result := Set{}
	for _, left := range set.ranges {
		for _, right := range other.ranges {
			low, high := left.low, left.high
			if compare(right.low, low) > 0 {
				low = right.low
			}
			if high == nil || right.high != nil && compare(*right.high, *high) < 0 {
				high = right.high
			}
			result.ranges = append(result.ranges, interval{low: low, high: high})
		}
	}
	return normalize(result)
}

func (set Set) Complement() Set {
	set = normalize(set)
	result, low := Set{}, version{}
	for _, span := range set.ranges {
		high := span.low
		result.ranges = append(result.ranges, interval{low: low, high: &high})
		if span.high == nil {
			return normalize(result)
		}
		low = *span.high
	}
	result.ranges = append(result.ranges, interval{low: low})
	return normalize(result)
}

func (set Set) MinorRange(minimum, maximum string) (bool, bool, error) {
	low, n, wild, err := parseVersion(minimum)
	if err != nil || n != 2 || wild {
		return false, false, fmt.Errorf("invalid PHP minor %q", minimum)
	}
	high, n, wild, err := parseVersion(maximum)
	if err != nil || n != 2 || wild || compare(low, high) > 0 {
		return false, false, fmt.Errorf("invalid PHP minor range")
	}
	start, end := minorKey(low), minorKey(high)
	cursor := start
	for _, interval := range set.ranges {
		first, last := minorKey(interval.low), ^uint64(0)
		if interval.high != nil {
			last = minorKey(*interval.high)
			if interval.high[2] == 0 {
				if last == 0 {
					continue
				}
				last--
			}
		}
		if last < cursor {
			continue
		}
		if first > cursor {
			return false, false, nil
		}
		if last >= end {
			upper := next(high, 1)
			for _, candidate := range set.ranges {
				if compare(candidate.low, low) <= 0 && (candidate.high == nil || upper != nil && compare(*candidate.high, *upper) >= 0) {
					return true, true, nil
				}
			}
			return true, false, nil
		}
		cursor = last + 1
	}
	return false, false, nil
}

func normalize(set Set) Set {
	filtered := []interval{}
	for _, interval := range set.ranges {
		if interval.high == nil || compare(interval.low, *interval.high) < 0 {
			filtered = append(filtered, interval)
		}
	}
	slices.SortFunc(filtered, func(a, b interval) int { return compare(a.low, b.low) })
	result := Set{}
	for _, interval := range filtered {
		if len(result.ranges) == 0 {
			result.ranges = append(result.ranges, interval)
			continue
		}
		last := &result.ranges[len(result.ranges)-1]
		if last.high == nil {
			continue
		}
		if compare(interval.low, *last.high) <= 0 {
			if interval.high == nil || compare(*interval.high, *last.high) > 0 {
				last.high = interval.high
			}
		} else {
			result.ranges = append(result.ranges, interval)
		}
	}
	return result
}

func next(value version, index int) *version {
	for i := index + 1; i < 3; i++ {
		value[i] = 0
	}
	for i := index; i >= 0; i-- {
		if value[i] < maxPart {
			value[i]++
			return &value
		}
		value[i] = 0
	}
	return nil
}

func compare(a, b version) int {
	for i := range a {
		if value := cmp.Compare(a[i], b[i]); value != 0 {
			return value
		}
	}
	return 0
}

func minorKey(value version) uint64 { return value[0]<<32 | value[1] }
