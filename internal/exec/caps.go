package exec

import (
	"context"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
)

// Caps can only reduce common execution limits for a concrete consumer. It has
// no invocation mode, authority, SQL, credential or cancellation-grace override.
type Caps struct {
	Rows int
	Bytes int
	Timeout time.Duration
	PlannerCost float64
}
func(c Caps)valid()bool{return c.Rows>0&&c.Rows<=100000&&c.Bytes>=128&&c.Bytes<=16<<20&&c.Timeout>=time.Millisecond&&c.Timeout<=time.Minute&&c.PlannerCost>0&&c.PlannerCost<=1e12}

// Execute applies the common configured ceiling to a validator-issued plan.
func(x *Executor)Execute(ctx context.Context,e identity.Envelope,p Plan,o Options)(ExecutionReport,error){return x.execute(ctx,e,p,o,nil)}

// ExecuteCapped uses the SAME slots, journal, cancellation and native path as
// Execute. Profiling cannot escape global concurrency or increase any ceiling.
func(x *Executor)ExecuteCapped(ctx context.Context,e identity.Envelope,p Plan,o Options,c Caps)(ExecutionReport,error){
	if !c.valid(){return ExecutionReport{},ErrLimit}
	return x.execute(ctx,e,p,o,&c)
}
