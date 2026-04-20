package httpretry

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		status int
		want   Category
	}{
		{200, CategoryNone},
		{201, CategoryNone},
		{400, CategoryNone},
		{401, CategoryNone},
		{404, CategoryNone},
		{429, CategoryRateLimit},
		{408, CategoryTimeout},
		{504, CategoryTimeout},
		{500, CategoryTransient},
		{502, CategoryTransient},
		{503, CategoryTransient},
		{529, CategoryTransient},
	}
	for _, tc := range cases {
		got := Classify(tc.status)
		if got != tc.want {
			t.Errorf("Classify(%d) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestIsRetryable(t *testing.T) {
	retryable := []int{429, 408, 500, 502, 503, 504, 529}
	for _, s := range retryable {
		if !IsRetryable(s) {
			t.Errorf("IsRetryable(%d) = false, want true", s)
		}
	}
	notRetryable := []int{200, 201, 400, 401, 403, 404, 422}
	for _, s := range notRetryable {
		if IsRetryable(s) {
			t.Errorf("IsRetryable(%d) = true, want false", s)
		}
	}
}
