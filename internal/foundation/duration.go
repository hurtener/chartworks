package foundation

import (
 "time"
 "github.com/hurtener/chartworks/internal/config"
)

func timeDuration(d config.Duration)time.Duration{return time.Duration(d)}
