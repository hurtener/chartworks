package reporting

import "time"

// ScheduledProvenance is content-free metadata from the accepted occurrence.
// It exposes neither issuer bindings, recipients, actor identities, parameters,
// SQL nor tokens. Delivery stages are independent: retained query values do not
// imply catalog publication or any outbound notification. This is not a grant.
type ScheduledProvenance struct {
	ScheduleID       string     `json:"schedule_id,omitempty"`
	ScheduleRevision int64      `json:"schedule_revision,omitempty"`
	DueAt            time.Time  `json:"due_at"`
	WindowStart      time.Time  `json:"window_start"`
	WindowEnd        time.Time  `json:"window_end"`
	Execution        string     `json:"execution"`
	Query            string     `json:"query"`
	Artifact         string     `json:"artifact"`
	Catalog          string     `json:"catalog"`
	Notification     string     `json:"notification"`
	PublishedAt      *time.Time `json:"published_at,omitempty"`
}
