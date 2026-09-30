package nlqexec

import (
	"encoding/json"
	"github.com/hurtener/chartworks/internal/exec"
)

// The registered engine function profile remains the generation vocabulary.
// These instructions describe only calendar shapes proved by the corresponding
// native analytical adapter, with the exact compiled bucket contract attached.
func analyticalGrainGuidanceForDialect(dialect string, c *exec.AnalyticalContract) string {
	if dialect == "postgres" || c == nil || c.Grain == nil || len(c.Grain.Buckets) == 0 {
		return analyticalGrainGuidance(c)
	}
	raw, err := json.Marshal(c.Grain)
	if err != nil {
		return ""
	}
	syntax := ""
	switch dialect {
	case "mysql":
		syntax = " For civil/date inputs use CAST(DATE_FORMAT(field,'%Y-%m-01') AS DATE) for month, '%Y-%m-%d' for day, '%Y-01-01' for year. For quarter use MAKEDATE(EXTRACT(YEAR FROM field),1) + INTERVAL EXTRACT(QUARTER FROM field)-1 QUARTER. Session-sensitive TIMESTAMP inputs require separately proved timezone handling."
	case "sqlserver":
		syntax = " Use DATETRUNC(unit,field) with the registered unquoted date-part keyword for date/civil inputs. Do not treat datetimeoffset values as a fixed reviewed timezone without explicit proved conversion."
	case "bigquery":
		syntax = " Use DATE_TRUNC(date_field,UNIT), DATETIME_TRUNC(datetime_field,UNIT), or TIMESTAMP_TRUNC(timestamp_field,UNIT,'exact reviewed timezone'); UNIT is the registered unquoted date-part keyword."
	case "snowflake", "databricks":
		syntax = " Use DATE_TRUNC('unit',date_or_timestamp_ntz_field). Do not assume a session timezone for timestamp values."
	}
	return " The exact selected grouping is enforced: project and GROUP BY every reviewed direct field and complete calendar bucket, preserving years and NULL groups." + syntax + " Grouping contract: " + string(raw) + analyticalPopulationGuidance(c) + analyticalIntentGuidance(c)
}
