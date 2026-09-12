package config

import (
	"github.com/hurtener/chartworks/internal/calendars"
	"testing"
)

func TestJobsTimezoneDatabaseVersion(t *testing.T) {
	j := DefaultJobs()
	if j.TimezoneDatabaseVersion != calendars.Version || ValidateJobs(j, Auth{}) != nil {
		t.Fatal("pinned default")
	}
	j.TimezoneDatabaseVersion = "unknown"
	if ValidateJobs(j, Auth{}) == nil {
		t.Fatal("accepted another database")
	}
	j.TimezoneDatabaseVersion = ""
	if ValidateJobs(j, Auth{}) != nil {
		t.Fatal("omission must use the bundled default")
	}
}
