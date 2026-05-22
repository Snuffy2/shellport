// Copyright (C) 2019-2026 Ni Rui <ranqus@gmail.com>
// Copyright (C) 2026 Snuffy2
// SPDX-License-Identifier: AGPL-3.0-only

package configuration

import (
	"fmt"

	"io"
	"os"
	"path/filepath"
	"strings"
)

// GetEnv looks up the environment variable named name.
func GetEnv(name string) string {
	return os.Getenv(name)
}

// String is a configuration string that may contain a URI scheme to indicate
// how its value should be resolved. Plain strings are returned as-is; strings
// beginning with "file://", "environment://", or "literal://" are resolved
// through their respective sources.
type String string

// Parse resolves the configuration string and returns the final string value.
// Supported schemes:
//   - file://path    reads the file at path and returns its contents as a string.
//   - environment://name returns the value of the named environment variable.
//   - literal://value returns value verbatim, preserving any embedded "://" sequences.
//
// Strings without a recognised scheme are returned unchanged. Unknown schemes
// return an error.
func (s String) Parse() (string, error) {
	ss := string(s)
	sSchemeLeadIdx := strings.Index(ss, "://")
	if sSchemeLeadIdx < 0 {
		return ss, nil
	}
	sSchemeLeadEnd := sSchemeLeadIdx + 3
	switch strings.ToLower(ss[:sSchemeLeadIdx]) {
	case "file":
		fPath, e := filepath.Abs(ss[sSchemeLeadEnd:])
		if e != nil {
			return ss, e
		}
		f, e := os.Open(fPath)
		if e != nil {
			return "", fmt.Errorf("unable to open %s: %s", fPath, e)
		}
		defer f.Close()
		fData, e := io.ReadAll(f)
		if e != nil {
			return "", fmt.Errorf("unable to read from %s: %s", fPath, e)
		}
		return string(fData), nil
	case "environment":
		return GetEnv(ss[sSchemeLeadEnd:]), nil
	case "literal":
		return ss[sSchemeLeadEnd:], nil
	default:
		return "", fmt.Errorf(
			"scheme \"%s\" was unsupported", ss[:sSchemeLeadIdx])
	}
}
