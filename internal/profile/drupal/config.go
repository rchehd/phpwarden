package drupal

import (
	"path"
	"strings"
)

func ModuleConfigFile(filename string, directories, moduleRoots []string) bool {
	extension := strings.ToLower(path.Ext(filename))
	if extension != ".yml" && extension != ".yaml" {
		return false
	}
	for _, directory := range directories {
		directory = path.Clean(directory)
		parent := path.Dir(filename)
		for parent != "." {
			module := ""
			if parent == directory {
				module = "."
			} else if strings.HasSuffix(parent, "/"+directory) {
				module = strings.TrimSuffix(parent, "/"+directory)
			}
			if module != "" {
				for _, root := range moduleRoots {
					if path.Clean(root) == module {
						return true
					}
				}
				parts := strings.Split(module, "/")
				for i, part := range parts {
					if part != "modules" {
						continue
					}
					rest := parts[i+1:]
					if len(rest) == 1 && rest[0] != "custom" && rest[0] != "contrib" ||
						len(rest) == 2 && (rest[0] == "custom" || rest[0] == "contrib") {
						return true
					}
				}
			}
			parent = path.Dir(parent)
		}
	}
	return false
}
