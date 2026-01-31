package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	configPath := flag.String("config", "build/config.yml", "path to build/config.yml")
	flag.Parse()

	f, err := os.Open(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	version, err := readInfoVersion(f, *configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(version)
}

func readInfoVersion(r *os.File, configPath string) (string, error) {
	scanner := bufio.NewScanner(r)

	inInfo := false
	for scanner.Scan() {
		line := scanner.Text()
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}

		indent := countIndent(line)

		// Enter info: block (must be top-level)
		if !inInfo {
			if indent == 0 && (trim == "info:" || strings.HasPrefix(trim, "info:")) {
				inInfo = true
			}
			continue
		}

		// Exit info: block when a new top-level key starts
		if indent == 0 {
			break
		}

		// Parse "version:" under info:
		if strings.HasPrefix(trim, "version:") {
			raw := strings.TrimSpace(strings.TrimPrefix(trim, "version:"))
			v := parseYAMLScalar(raw)
			if v == "" {
				return "", fmt.Errorf("failed to parse info.version from %q", configPath)
			}
			return v, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("info.version not found in %q", configPath)
}

func countIndent(s string) int {
	n := 0
	for len(s) > 0 {
		switch s[0] {
		case ' ':
			n++
			s = s[1:]
		case '\t':
			// treat tab as one level for our simple use
			n++
			s = s[1:]
		default:
			return n
		}
	}
	return n
}

// parseYAMLScalar extracts a scalar value from a YAML token, stripping optional quotes and inline comments.
// It is intentionally small: enough to parse lines like:
//
//	version: "0.0.1" # comment
//	version: 0.0.1
func parseYAMLScalar(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// Quoted strings
	if strings.HasPrefix(s, "\"") {
		s = s[1:]
		if i := strings.IndexByte(s, '"'); i >= 0 {
			return strings.TrimSpace(s[:i])
		}
		return strings.TrimSpace(s)
	}
	if strings.HasPrefix(s, "'") {
		s = s[1:]
		if i := strings.IndexByte(s, '\''); i >= 0 {
			return strings.TrimSpace(s[:i])
		}
		return strings.TrimSpace(s)
	}

	// Unquoted: strip inline comment
	if i := strings.IndexByte(s, '#'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
