package runtime

import (
	"strings"
	"testing"
)

// FuzzConfigParse tests ParseConfig with arbitrary YAML-like inputs.
func FuzzConfigParse(f *testing.F) {
	seeds := []string{
		`name: test`,
		`name: test\nport: 8080`,
		`{invalid yaml`,
		``,
		`name: test\ndatabases:\n  - name: db\n    url: "postgres://..."`,
		`entry:\n  - type: rest\n    method: GET\n    path: /test\n    handler: h`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		cfg, err := ParseConfig(data)
		if err != nil {
			return
		}
		if cfg == nil {
			return
		}
	})
}

// FuzzSanitizeKey tests sanitizeKey with various path-like inputs.
func FuzzSanitizeKey(f *testing.F) {
	seeds := []string{
		"normal/file.txt",
		"../../../etc/passwd",
		"/absolute/path",
		"..\\..\\windows\\system32",
		"\x00null\x00byte",
		strings.Repeat("a", 10000),
		"valid-key",
		"path/to/file.go",
		"../relative",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, key string) {
		sanitized := sanitizeKey(key)
		if strings.Contains(sanitized, "..") {
			t.Errorf("path traversal not sanitized: %q → %q", key, sanitized)
		}
		if strings.HasPrefix(sanitized, "/") {
			t.Errorf("absolute path not sanitized: %q → %q", key, sanitized)
		}
	})
}

// FuzzParseMaxSize tests parseMaxSize with arbitrary size strings.
func FuzzParseMaxSize(f *testing.F) {
	seeds := []string{
		"1KB",
		"10MB",
		"1GB",
		"100",
		"0",
		"-1",
		"abc",
		"1.5MB",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, size string) {
		result := parseMaxSize(size)
		if result < 0 {
			t.Errorf("parseMaxSize(%q) = %d, expected >= 0", size, result)
		}
	})
}
