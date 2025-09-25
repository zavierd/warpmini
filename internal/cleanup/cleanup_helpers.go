package cleanup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Errors struct {
	messages []string
}

func (c *Errors) Add(path string, err error) {
	if err == nil {
		return
	}
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	c.messages = append(c.messages, fmt.Sprintf("%s: %v", path, err))
}

func (c *Errors) Merge(err error) {
	if err == nil {
		return
	}
	c.messages = append(c.messages, err.Error())
}

func (c *Errors) Err() error {
	if len(c.messages) == 0 {
		return nil
	}
	return fmt.Errorf(strings.Join(c.messages, "; "))
}

func expandPath(path string) string {
	if path == "" {
		return ""
	}
	expanded := os.ExpandEnv(path)
	if strings.HasPrefix(expanded, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			switch {
			case expanded == "~":
				expanded = home
			case strings.HasPrefix(expanded, "~/"):
				expanded = filepath.Join(home, expanded[2:])
			default:
				expanded = filepath.Join(home, expanded[1:])
			}
		}
	}
	return filepath.Clean(expanded)
}

func removePath(path string) error {
	cleaned := expandPath(path)
	if cleaned == "" {
		return nil
	}
	if _, err := os.Lstat(cleaned); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return os.RemoveAll(cleaned)
}

func RemovePaths(paths []string, errs *Errors) {
	for _, path := range paths {
		cleaned := expandPath(path)
	errs.Add(cleaned, removePath(cleaned))
	}
}

func RemoveGlob(pattern string, errs *Errors) {
	expanded := expandPath(pattern)
	matches, err := filepath.Glob(expanded)
	if err != nil {
		errs.Merge(fmt.Errorf("glob %s: %w", expanded, err))
		return
	}
	for _, match := range matches {
		errs.Add(match, removePath(match))
	}
}
