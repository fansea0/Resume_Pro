// Package envfile loads local KEY=VALUE settings from a .env file into the
// process environment without pulling in an external dependency.
//
// Rules:
//   - variables already present in the environment win, so an explicit
//     `export OPENAI_API_KEY=...` in the shell overrides the file;
//   - values are trimmed and surrounding single/double quotes are removed;
//   - blank lines and `#` comments are ignored.
package envfile

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Parse reads KEY=VALUE pairs from r and returns them in a map.
func Parse(r *bufio.Reader) (map[string]string, error) {
	values := make(map[string]string)
	for lineNumber := 1; ; lineNumber++ {
		line, err := r.ReadString('\n')
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			key, value, ok := strings.Cut(trimmed, "=")
			if !ok {
				return nil, fmt.Errorf("line %d: expected KEY=VALUE", lineNumber)
			}
			key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
			if key == "" {
				return nil, fmt.Errorf("line %d: empty variable name", lineNumber)
			}
			values[key] = unquote(strings.TrimSpace(value))
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("read env file: %w", err)
		}
	}
	return values, nil
}

// Load applies the pairs found in path to the process environment. Variables
// that are already set are left untouched. A missing file is not an error.
func Load(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open env file %s: %w", path, err)
	}
	defer file.Close()

	values, err := Parse(bufio.NewReader(file))
	if err != nil {
		return fmt.Errorf("parse env file %s: %w", path, err)
	}
	for key, value := range values {
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}
	return nil
}

// LoadFromPaths loads the first .env file that exists among the given
// directories and returns its path. It returns an empty path when none of the
// directories contains a .env file.
func LoadFromPaths(directories []string) (string, error) {
	for _, directory := range directories {
		path := directory + string(os.PathSeparator) + ".env"
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("stat env file %s: %w", path, err)
		}
		if info.IsDir() {
			continue
		}
		if err := Load(path); err != nil {
			return "", err
		}
		return path, nil
	}
	return "", nil
}

func unquote(value string) string {
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}
