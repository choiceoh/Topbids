// Package httpretry provides HTTP status code retry classification.
// Ported from Deneb gateway.
package httpretry

// Category classifies an HTTP status code for retry decisions.
type Category int

const (
	CategoryNone Category = iota
	CategoryTransient
	CategoryTimeout
	CategoryRateLimit
)

func Classify(status int) Category {
	switch status {
	case 429:
		return CategoryRateLimit
	case 408, 504:
		return CategoryTimeout
	case 500, 502, 503, 529:
		return CategoryTransient
	default:
		return CategoryNone
	}
}

func IsRetryable(status int) bool {
	return Classify(status) != CategoryNone
}
