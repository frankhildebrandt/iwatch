package watch

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/git-pkgs/gitignore"
)

type ignoreMatcher struct {
	root    string
	matcher *gitignore.Matcher
}

func newIgnoreMatcher(root string) (*ignoreMatcher, error) {
	m := gitignore.New("")

	if err := loadGitignoreTree(m, root); err != nil {
		return nil, err
	}

	// `.iwatchignore` has higher priority than `.gitignore`.
	if err := addIgnoreFileIfExists(m, filepath.Join(root, ".iwatchignore"), ""); err != nil {
		return nil, err
	}

	return &ignoreMatcher{root: root, matcher: m}, nil
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

	return m.matcher.MatchPath(relative, isDir)
}

func loadGitignoreTree(m *gitignore.Matcher, root string) error {
	return filepath.WalkDir(root, func(currentPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}

		if d.IsDir() {
			return nil
		}

		if d.Name() != ".gitignore" {
			return nil
		}

		dir := filepath.Dir(currentPath)
		scope, err := filepath.Rel(root, dir)
		if err != nil {
			return fmt.Errorf("compute .gitignore scope for %s: %w", currentPath, err)
		}
		if scope == "." {
			scope = ""
		}
		scope = filepath.ToSlash(scope)

		if err := addIgnoreFileIfExists(m, currentPath, scope); err != nil {
			return err
		}
		return nil
	})
}

func addIgnoreFileIfExists(m *gitignore.Matcher, filePath string, scope string) error {
	_, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", filePath, err)
	}

	m.AddFromFile(filePath, scope)
	return nil
}
