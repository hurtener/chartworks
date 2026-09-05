// Package telemetry records bounded content-free events and owns its metrics registry.
package telemetry

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Event is a closed vocabulary: no caller-supplied message or attribute can reach the logger.
type Event string
const (
	// Started is emitted after the listener has bound.
	Started Event = "started"
	// Stopped is emitted after server and monitors have joined.
	Stopped Event = "stopped"
	// ConfigurationRejected records safe validation failure, never its input.
	ConfigurationRejected Event = "configuration_rejected"
	// Request records one bounded-route health request.
	Request Event = "request"
	// DependencyChanged records a dependency state transition.
	DependencyChanged Event = "dependency_changed"
	// AuditCommitted records committed durable metadata, without its contents.
	AuditCommitted Event = "audit_committed"
)
// Reporter is safe for concurrent use. Callers never obtain its mutable registry/logger.
type Reporter struct {
	logger *slog.Logger
	registry *prometheus.Registry
	events *prometheus.CounterVec
	ready *prometheus.GaugeVec
}
// New constructs and initializes every metric series, including zero-valued counters.
func New(w io.Writer,format string)(*Reporter,error) {
	if w==nil { return nil,errors.New("telemetry: writer required") }
	var h slog.Handler
	switch format { case "json": h=slog.NewJSONHandler(w,nil);case "text":h=slog.NewTextHandler(w,nil);default:return nil,errors.New("telemetry: unsupported log format") }
	r:=&Reporter{logger:slog.New(h),registry:prometheus.NewRegistry(),events:prometheus.NewCounterVec(prometheus.CounterOpts{Name:"chartworks_events_total",Help:"Content-free foundation events."},[]string{"event","outcome"}),ready:prometheus.NewGaugeVec(prometheus.GaugeOpts{Name:"chartworks_dependency_ready",Help:"Required dependency readiness."},[]string{"dependency"})}
	if err:=r.registry.Register(r.events);err!=nil{return nil,errors.New("telemetry: metric registration failed")}
	if err:=r.registry.Register(r.ready);err!=nil{return nil,errors.New("telemetry: metric registration failed")}
	for _,e:=range []Event{Started,Stopped,ConfigurationRejected,Request,DependencyChanged,AuditCommitted} { for _,o:=range []string{"ok","error"} { r.events.WithLabelValues(string(e),o).Add(0) } }
	for _,d:=range []string{"store","verification_keys"} { r.ready.WithLabelValues(d).Set(0) }
	return r,nil
}
// Record rejects unknown event values without echoing them.
func (r *Reporter) Record(ctx context.Context,event Event,success bool) error {
	switch event {case Started,Stopped,ConfigurationRejected,Request,DependencyChanged,AuditCommitted:default:return errors.New("telemetry: unknown event")}
	outcome:="error";if success {outcome="ok"}
	r.events.WithLabelValues(string(event),outcome).Inc()
	// High-frequency request data remains in counters, not an unbounded log stream.
	if event!=Request { r.logger.LogAttrs(ctx,slog.LevelInfo,"chartworks",slog.String("event",string(event)),slog.String("outcome",outcome)) }
	return nil
}
// Dependency accepts only fixed dependency labels.
func (r *Reporter) Dependency(name string,ready bool) error {
	if name!="store"&&name!="verification_keys" {return errors.New("telemetry: unknown dependency")}
	value:=0.;if ready {value=1};r.ready.WithLabelValues(name).Set(value);return nil
}
// Handler is the real Prometheus exporter. It is not installed on an unauthenticated business listener.
func (r *Reporter) Handler() http.Handler { return promhttp.HandlerFor(r.registry,promhttp.HandlerOpts{}) }
// MetricNames is the closed exporter-conformance inventory.
func MetricNames() []string { return []string{"chartworks_events_total","chartworks_dependency_ready"} }
