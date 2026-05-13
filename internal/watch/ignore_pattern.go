package watch

import (
	"path"
	"strings"
)

type ignorePattern struct {
	raw           string
	negated       bool
	directoryOnly bool
	anchored      bool
	basenameOnly  bool
}

func (p ignorePattern) matches(relativePath string, isDir bool) bool {
	if p.raw == "" {
		return false
	}
	if p.directoryOnly && !isDir {
		return false
	}

	candidates := buildIgnoreCandidates(relativePath, p)
	for _, candidate := range candidates {
		matched, err := path.Match(p.raw, candidate)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func buildIgnoreCandidates(relativePath string, pattern ignorePattern) []string {
	if pattern.anchored {
		return []string{relativePath}
	}
	if pattern.basenameOnly {
		parts := strings.Split(relativePath, "/")
		return parts
	}

	candidates := []string{relativePath}
	for current := relativePath; ; {
		index := strings.IndexByte(current, '/')
		if index == -1 {
			break
		}
		current = current[index+1:]
		candidates = append(candidates, current)
	}
	return candidates
}
