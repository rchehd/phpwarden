package scope

import (
	"path"
	"strings"
)

func matchGlob(pattern, filename string) bool {
	patternParts := strings.Split(pattern, "/")
	fileParts := strings.Split(filename, "/")
	type position struct{ pattern, file int }
	cache := make(map[position]bool)
	visited := make(map[position]bool)
	var match func(int, int) bool
	match = func(p, f int) bool {
		key := position{p, f}
		if visited[key] {
			return cache[key]
		}
		visited[key] = true
		result := false
		if p == len(patternParts) {
			result = f == len(fileParts)
		} else if patternParts[p] == "**" {
			result = match(p+1, f) || f < len(fileParts) && match(p, f+1)
		} else if f < len(fileParts) {
			matched, _ := path.Match(patternParts[p], fileParts[f])
			result = matched && match(p+1, f+1)
		}
		cache[key] = result
		return result
	}
	return match(0, 0)
}

func inRoots(filename string, roots []string) bool {
	for _, root := range roots {
		if root == "." || filename == root || strings.HasPrefix(filename, root+"/") {
			return true
		}
	}
	return false
}
