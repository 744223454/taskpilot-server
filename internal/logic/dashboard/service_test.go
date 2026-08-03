package dashboard

import (
	"testing"
	"time"
)

func TestDaysLeftRoundsUpPartialDays(t *testing.T) {
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)

	for _, testCase := range []struct {
		name     string
		deadline time.Time
		want     int
	}{
		{name: "due now", deadline: now, want: 0},
		{name: "due in one hour", deadline: now.Add(time.Hour), want: 1},
		{name: "due in one day", deadline: now.Add(24 * time.Hour), want: 1},
		{name: "due just over one day", deadline: now.Add(24*time.Hour + time.Second), want: 2},
		{name: "overdue", deadline: now.Add(-time.Hour), want: 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := daysLeft(now, testCase.deadline); got != testCase.want {
				t.Fatalf("daysLeft() = %d, want %d", got, testCase.want)
			}
		})
	}
}
