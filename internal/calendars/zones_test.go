package calendars

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"testing"
	"time"
)

func TestPinnedTimezoneArchive(t *testing.T) {
	sum := sha256.Sum256(archive)
	if Version != "go1.26.4-sha256:"+hex.EncodeToString(sum[:]) {
		t.Fatal("archive/version mismatch")
	}
	t.Setenv("ZONEINFO", "/does/not/exist")
	zone, err := Location("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	before := time.Date(2024, 3, 10, 6, 59, 59, 0, time.UTC).In(zone)
	after := before.Add(time.Second)
	if before.Hour() != 1 || after.Hour() != 3 {
		t.Fatal("spring transition", before, after)
	}
	first := time.Date(2024, 11, 3, 5, 30, 0, 0, time.UTC).In(zone)
	second := first.Add(time.Hour)
	_, a := first.Zone()
	_, b := second.Zone()
	if first.Hour() != 1 || second.Hour() != 1 || a == b {
		t.Fatal("fold transition", first, second)
	}
	for _, name := range []string{"", "Local", "../UTC", "/etc/passwd", "UTC\x00", "missing", "Etc/../UTC", "America\\New_York"} {
		if _, err := Location(name); err == nil {
			t.Fatalf("accepted unsafe/absent zone %q", name)
		}
	}
	if zone, err := Location("UTC"); err != nil || zone != time.UTC {
		t.Fatal("UTC", err)
	}
}
func TestTimezoneConcurrentReuse(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			zone, err := Location("America/Argentina/Buenos_Aires")
			if err != nil || zone == nil {
				t.Error("concurrent location", err)
			}
		}()
	}
	wg.Wait()
}
func FuzzTimezoneNames(f *testing.F) {
	for _, name := range []string{"UTC", "Europe/Paris", "Australia/Lord_Howe", "../UTC", ""} {
		f.Add(name)
	}
	f.Fuzz(func(t *testing.T, name string) {
		zone, err := Location(name)
		if err == nil && zone == nil {
			t.Fatal("nil accepted zone")
		}
	})
}
