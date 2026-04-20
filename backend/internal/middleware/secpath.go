// Security path canonicalization — multi-pass URL decode + normalization.
// Ported from Deneb auth/security_path.go (a8f633aff3).
package middleware

import (
	"net/url"
	"path"
	"strings"
)

const maxPathDecodePasses = 32

type PathCanonicalization struct {
	CanonicalPath          string
	Candidates             []string
	DecodePasses           int
	DecodePassLimitReached bool
	MalformedEncoding      bool
}

// CanonicalizePath performs multi-pass URL decode with normalization.
// Fail-closed: collects all intermediate forms for security checks.
func CanonicalizePath(pathname string) PathCanonicalization {
	result := PathCanonicalization{}
	current := pathname
	seen := make(map[string]bool)

	for i := range maxPathDecodePasses {
		normalized := normalizePath(current)
		if !seen[normalized] {
			seen[normalized] = true
			result.Candidates = append(result.Candidates, normalized)
		}

		decoded, err := url.PathUnescape(current)
		if err != nil {
			result.MalformedEncoding = true
			break
		}
		if decoded == current {
			break
		}
		current = decoded
		result.DecodePasses = i + 1

		if i == maxPathDecodePasses-1 {
			nextDecoded, _ := url.PathUnescape(current)
			if nextDecoded != current {
				result.DecodePassLimitReached = true
			}
		}
	}

	if len(result.Candidates) > 0 {
		result.CanonicalPath = result.Candidates[len(result.Candidates)-1]
	} else {
		result.CanonicalPath = normalizePath(pathname)
	}
	return result
}

// IsProtectedPath checks if any decoded form of the path matches protected prefixes.
// Fail-closed on decode limit.
func IsProtectedPath(pathname string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return false
	}
	canon := CanonicalizePath(pathname)
	if canon.DecodePassLimitReached {
		return true
	}

	normalizedPrefixes := make([]string, len(prefixes))
	for i, p := range prefixes {
		normalizedPrefixes[i] = normalizePath(p)
	}

	for _, candidate := range canon.Candidates {
		for _, prefix := range normalizedPrefixes {
			if matchesPrefix(candidate, prefix) {
				return true
			}
		}
	}
	return false
}

func normalizePath(p string) string {
	p = strings.ToLower(p)
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		p = p[:len(p)-1]
	}
	p = path.Clean(p)
	if p == "." {
		p = "/"
	}
	return p
}

func matchesPrefix(pathname, prefix string) bool {
	if pathname == prefix {
		return true
	}
	if strings.HasPrefix(pathname, prefix+"/") {
		return true
	}
	if strings.HasPrefix(pathname, prefix+"%") {
		return true
	}
	return false
}
