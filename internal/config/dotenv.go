package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// ReadDotEnv parses a .env file. A missing file returns (nil map, nil).
//
// Format: KEY=VALUE per line; blank lines and lines starting with # are
// skipped; an optional "export " prefix is stripped; key and value are
// trimmed; a value wrapped in matching single or double quotes loses the
// quotes. There is no escape processing, no ${VAR} interpolation and no
// inline comments.
func ReadDotEnv(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	out := make(map[string]string)
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "export "); ok {
			line = strings.TrimSpace(rest)
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("%s:%d: expected KEY=VALUE", path, i+1)
		}
		out[key] = unquote(strings.TrimSpace(value))
	}
	return out, nil
}

func unquote(v string) string {
	if len(v) >= 2 {
		if q := v[0]; (q == '"' || q == '\'') && v[len(v)-1] == q {
			return v[1 : len(v)-1]
		}
	}
	return v
}
