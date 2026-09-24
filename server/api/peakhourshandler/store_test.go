package peakhourshandler

import (
	"errors"
	"testing"
)

func TestValidate_ValidDeepSeekExample(t *testing.T) {
	p := deepSeekExample()
	if err := validate(&p); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestValidate_MissingName(t *testing.T) {
	p := deepSeekExample()
	p.Name = "   "
	if err := validate(&p); !errors.Is(err, ErrPeakHoursInvalid) {
		t.Fatalf("err = %v, want ErrPeakHoursInvalid", err)
	}
}

func TestValidate_NoHours(t *testing.T) {
	p := deepSeekExample()
	p.Hours = nil
	if err := validate(&p); !errors.Is(err, ErrPeakHoursInvalid) {
		t.Fatalf("err = %v, want ErrPeakHoursInvalid", err)
	}
}

func TestValidate_BadHourRange(t *testing.T) {
	cases := []string{"9:00-12:00", "09:00", "09:00-08:00", "25:00-26:00", "09:00-09:00"}
	for _, hours := range cases {
		p := deepSeekExample()
		p.Hours = []string{hours}
		if err := validate(&p); !errors.Is(err, ErrPeakHoursInvalid) {
			t.Errorf("hours=%q: err = %v, want ErrPeakHoursInvalid", hours, err)
		}
	}
}

func TestValidate_InvalidTimezone(t *testing.T) {
	p := deepSeekExample()
	p.Timezone = "Beijing time"
	if err := validate(&p); !errors.Is(err, ErrPeakHoursInvalid) {
		t.Fatalf("err = %v, want ErrPeakHoursInvalid", err)
	}
}

func TestValidate_ApplicableDaysModes(t *testing.T) {
	cases := []struct {
		name    string
		ad      ApplicableDays
		wantErr bool
	}{
		{"workdays ok", ApplicableDays{Mode: "workdays"}, false},
		{"weekdays ok", ApplicableDays{Mode: "weekdays", Days: []interface{}{"mon", "fri"}}, false},
		{"weekdays empty", ApplicableDays{Mode: "weekdays"}, true},
		{"weekdays bad name", ApplicableDays{Mode: "weekdays", Days: []interface{}{"funday"}}, true},
		{"days_of_month ok", ApplicableDays{Mode: "days_of_month", Days: []interface{}{float64(1), float64(31)}}, false},
		{"days_of_month out of range", ApplicableDays{Mode: "days_of_month", Days: []interface{}{float64(32)}}, true},
		{"days_of_month not whole", ApplicableDays{Mode: "days_of_month", Days: []interface{}{float64(1.5)}}, true},
		{"unknown mode", ApplicableDays{Mode: "sometimes"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := deepSeekExample()
			p.ApplicableDays = c.ad
			err := validate(&p)
			if c.wantErr && !errors.Is(err, ErrPeakHoursInvalid) {
				t.Fatalf("err = %v, want ErrPeakHoursInvalid", err)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
		})
	}
}

func TestValidate_ExcludeDaysEntries(t *testing.T) {
	cases := []struct {
		entry   string
		wantErr bool
	}{
		{"weekends", false},
		{"holidays", false},
		{"2026-12-25", false},
		{"2026-12-24..2026-12-31", false},
		{"not-a-date", true},
		{"2026-13-01", true},
		{"2026-12-31..2026-12-24", false}, // parses fine even if reversed; matching just won't ever hit
	}
	for _, c := range cases {
		t.Run(c.entry, func(t *testing.T) {
			p := deepSeekExample()
			p.ExcludeDays = []string{c.entry}
			err := validate(&p)
			if c.wantErr && !errors.Is(err, ErrPeakHoursInvalid) {
				t.Fatalf("err = %v, want ErrPeakHoursInvalid", err)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
		})
	}
}
