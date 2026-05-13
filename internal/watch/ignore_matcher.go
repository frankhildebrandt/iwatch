package watch

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ignoreMatcher struct {
	root     string
	patterns []ignorePattern
}

func newIgnoreMatcher(root string) (*ignoreMatcher, error) {
	patterns := make([]ignorePattern, 0)
	for _, name := range []string{".gitignore", ".iwatchignore"} {
		filePatterns, err := loadIgnorePatterns(filepath.Join(root, name))
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, filePatterns...)
	}
	return &ignoreMatcher{
		root:     root,
		patterns: patterns,
	}, nil
}

func loadIgnorePatterns(path string) ([]ignorePattern, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	patterns := make([]ignorePattern, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		pattern := parseIgnorePattern(scanner.Text())
		if pattern.raw == "" {
			continue
		}
		patterns = append(patterns, pattern)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return patterns, nil
}

func (m *ignoreMatcher) matches(path string, isDir bool) bool {
	if m == nil {
		return false
	}

	relative, err := filepath.Rel(m.root, path)
	if err != nil {
		return false
	}
	relative = filepath.ToSlash(relative)
	if relative == "." {
		return false
	}

	ignored := false
	for _, pattern := range m.patterns {
		if !pattern.matches(relative, isDir) {
			continue
		}
		ignored = !pattern.negated
	}
	return ignored
}

func parseIgnorePattern(line string) ignorePattern {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return ignorePattern{}
	}

	negated := strings.HasPrefix(trimmed, "!")
	if negated {
		trimmed = strings.TrimPrefix(trimmed, "!")
	}

	directoryOnly := strings.HasSuffix(trimmed, "/")
	trimmed = strings.TrimSuffix(trimmed, "/")
	anchored := strings.HasPrefix(trimmed, "/")
	trimmed = strings.TrimPrefix(trimmed, "/")
	if trimmed == "" {
		return ignorePattern{}
	}

	return ignorePattern{
		raw:           trimmed,
		negated:       negated,
		directoryOnly: directoryOnly,
		anchored:      anchored,
		basenameOnly:  !strings.Contains(trimmed, "/"),
	}
}
