package usagestats

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveSettingsRejectsOverlappingPeakWindows(t *testing.T) {
	for _, windows := range [][4]string{
		{"09:00", "12:00", "11:00", "14:00"},
		{"11:00", "14:00", "09:00", "12:00"},
		{"09:00", "18:00", "12:00", "13:00"},
		{"09:00", "12:00", "09:00", "12:00"},
		{"09:00", "16:00", "", ""}, // Missing second window becomes 14:00–18:00.
	} {
		t.Run(windows[0]+"/"+windows[2], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "usage.json")
			store := New(path)
			defer store.Close()
			before := store.Settings()
			settings := store.Settings()
			settings.Peak.Start1, settings.Peak.End1 = windows[0], windows[1]
			settings.Peak.Start2, settings.Peak.End2 = windows[2], windows[3]
			if err := store.SaveSettings(settings); !errors.Is(err, ErrInvalidUsageSettings) {
				t.Fatalf("expected validation failure, got %v", err)
			}
			if !reflect.DeepEqual(before, store.Settings()) {
				t.Fatal("invalid save changed settings")
			}
			if _, err := os.Stat(path + ".settings"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid save created settings file: %v", err)
			}
		})
	}
}

func TestPeakWindowsMayTouchOrBeReversed(t *testing.T) {
	for _, windows := range [][4]string{
		{"09:00", "12:00", "12:00", "18:00"},
		{"14:00", "18:00", "09:00", "12:00"},
	} {
		settings := DefaultSettings().Peak
		settings.Start1, settings.End1 = windows[0], windows[1]
		settings.Start2, settings.End2 = windows[2], windows[3]
		if err := validatePeakSettings(settings); err != nil {
			t.Fatal(err)
		}
	}
}

func TestClockMinutesRequiresCompleteHHMM(t *testing.T) {
	for _, value := range []string{"09:00suffix", "9:00", "09:0", " 09:00", "+9:00", "09:60", "24:00", "09:00\n"} {
		if _, err := clockMinutes(value); err == nil {
			t.Errorf("accepted malformed time %q", value)
		}
	}
	for value, want := range map[string]int{"00:00": 0, "09:00": 540, "23:59": 1439} {
		if got, err := clockMinutes(value); err != nil || got != want {
			t.Errorf("%s: got %d, %v; want %d", value, got, err, want)
		}
	}
}

func BenchmarkClockMinutes(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if got, err := clockMinutes("14:30"); err != nil || got != 870 {
			b.Fatal(got, err)
		}
	}
}
