package engineering

// UploadsEnabled reports configured managed-write availability, not warehouse health.
func (s *Service) UploadsEnabled() bool { return s != nil && s.values.Uploads.Enabled }

// ProfilingEnabled reports configured profile generation availability. Retained
// evidence remains readable when generation or its model provider is disabled.
func (s *Service) ProfilingEnabled() bool { return s != nil && s.values.Profiling.Enabled }

// UploadByteLimit is the maximum transport body size; the reserved manifest's
// exact byte length and checksum are independently enforced before parsing.
func (s *Service) UploadByteLimit() int64 { return s.values.Uploads.MaxBytes }
