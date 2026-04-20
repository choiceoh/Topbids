package automation

import (
	"context"
	"testing"
	"time"
)

func TestCronMatches(t *testing.T) {
	// 2024-03-15 09:30 Friday (weekday=5), UTC.
	now := time.Date(2024, 3, 15, 9, 30, 0, 0, time.UTC)

	tests := []struct {
		name string
		cron string
		tz   string
		want bool
	}{
		{"all wildcards", "* * * * *", "", true},
		{"exact match", "30 9 15 3 5", "", true},
		{"minute mismatch", "0 9 15 3 5", "", false},
		{"hour mismatch", "30 10 15 3 5", "", false},
		{"day mismatch", "30 9 14 3 5", "", false},
		{"month mismatch", "30 9 15 4 5", "", false},
		{"weekday mismatch", "30 9 15 3 1", "", false},
		{"step minute */15", "*/15 * * * *", "", true}, // 30 % 15 == 0
		{"step minute */7", "*/7 * * * *", "", false},  // 30 % 7 != 0
		{"step hour */3", "* */3 * * *", "", true},     // 9 % 3 == 0
		{"comma minute", "15,30,45 * * * *", "", true}, // 30 is in list
		{"comma minute miss", "15,45 * * * *", "", false},
		{"empty cron", "", "", false},
		{"invalid field count", "* * *", "", false},
		{"too many fields", "* * * * * *", "", false},

		// Timezone: 2024-03-15 09:30 UTC = 2024-03-15 18:30 KST.
		{"KST hour", "30 18 * * *", "Asia/Seoul", true},
		{"KST hour mismatch", "30 9 * * *", "Asia/Seoul", false},
		{"invalid timezone fallback to UTC", "30 9 * * *", "Invalid/TZ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cronMatches(tt.cron, tt.tz, now)
			if got != tt.want {
				t.Errorf("cronMatches(%q, %q) = %v, want %v", tt.cron, tt.tz, got, tt.want)
			}
		})
	}
}

func TestFieldMatches(t *testing.T) {
	tests := []struct {
		name  string
		field string
		value int
		min   int
		max   int
		want  bool
	}{
		{"wildcard", "*", 5, 0, 59, true},
		{"exact match", "5", 5, 0, 59, true},
		{"exact mismatch", "5", 6, 0, 59, false},
		{"step */5 match", "*/5", 10, 0, 59, true},
		{"step */5 mismatch", "*/5", 11, 0, 59, false},
		{"step */5 zero", "*/5", 0, 0, 59, true},
		{"comma match first", "1,3,5", 1, 0, 59, true},
		{"comma match middle", "1,3,5", 3, 0, 59, true},
		{"comma match last", "1,3,5", 5, 0, 59, true},
		{"comma mismatch", "1,3,5", 2, 0, 59, false},
		{"day of month", "15", 15, 1, 31, true},
		{"step day */2 from 1", "*/2", 1, 1, 31, true},  // (1-1)%2 == 0
		{"step day */2 from 2", "*/2", 2, 1, 31, false}, // (2-1)%2 == 1
		{"invalid value", "abc", 5, 0, 59, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fieldMatches(tt.field, tt.value, tt.min, tt.max)
			if got != tt.want {
				t.Errorf("fieldMatches(%q, %d, %d, %d) = %v, want %v",
					tt.field, tt.value, tt.min, tt.max, got, tt.want)
			}
		})
	}
}

func TestSplitFields(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"* * * * *", 5},
		{"0 9 * * 1", 5},
		{"*/5 * * * *", 5},
		{"  30   9   15   3   5  ", 5},
		{"* * *", 3},
		{"", 0},
		{"*\t*\t*\t*\t*", 5},
	}

	for _, tt := range tests {
		got := splitFields(tt.input)
		if len(got) != tt.want {
			t.Errorf("splitFields(%q) = %d fields, want %d", tt.input, len(got), tt.want)
		}
	}
}

func TestSplitComma(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"1,2,3", 3},
		{"5", 1},
		{"0,15,30,45", 4},
		{"", 0}, // empty string yields no parts
	}
	for _, tt := range tests {
		got := splitComma(tt.input)
		if len(got) != tt.want {
			t.Errorf("splitComma(%q) = %d parts, want %d", tt.input, len(got), tt.want)
		}
	}
}

func TestParseIntSafe(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"0", 0, false},
		{"42", 42, false},
		{"100", 100, false},
		{"abc", 0, true},
		{"12a", 0, true},
		{"", 0, false}, // edge: empty string parses to 0
	}
	for _, tt := range tests {
		var v int
		_, err := parseIntSafe(tt.input, &v)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseIntSafe(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if err == nil && v != tt.want {
			t.Errorf("parseIntSafe(%q) = %d, want %d", tt.input, v, tt.want)
		}
	}
}

func TestEmptyScheduleEvent(t *testing.T) {
	ev := emptyScheduleEvent("col-123")
	if ev.CollectionID != "col-123" {
		t.Errorf("CollectionID = %q, want col-123", ev.CollectionID)
	}
	if string(ev.Type) != TriggerSchedule {
		t.Errorf("Type = %q, want %q", ev.Type, TriggerSchedule)
	}
}

func TestNewScheduler(t *testing.T) {
	s := NewScheduler(nil, 5*time.Minute)
	if s.interval != 5*time.Minute {
		t.Errorf("interval = %v, want 5m", s.interval)
	}
	if s.cancel == nil {
		t.Error("cancel func should not be nil")
	}
}

func TestScheduler_StartStop(t *testing.T) {
	s := NewScheduler(nil, time.Hour)
	s.Start(context.Background())
	// Should not block or panic.
	s.Stop()
}
