package pkg

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"

	"github.com/moby/patternmatcher"
)

// newDockerIgnoreMatcher loads <buildDir>/.dockerignore and returns a
// predicate for context-relative paths that must be excluded from the build
// context. Returns (nil, nil) when no .dockerignore exists.
func newDockerIgnoreMatcher(buildDir string) (func(rel string) bool, error) {
	f, err := os.Open(filepath.Join(buildDir, ".dockerignore"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	pm, err := patternmatcher.New(lines)
	if err != nil {
		return nil, fmt.Errorf("parsing .dockerignore: %w", err)
	}

	return func(rel string) bool {
		ok, _ := pm.MatchesOrParentMatches(filepath.ToSlash(rel))
		return ok
	}, nil
}
