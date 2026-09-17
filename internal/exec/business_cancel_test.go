package exec

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"
)

// A deterministic, monotonic cancellation source exercises every observed Err
// checkpoint without sleeps, goroutine races, or dependence on machine speed.
type businessCancelCheckpoint struct {
	context.Context
	cancel    context.CancelFunc
	remaining atomic.Int64
}

func (c *businessCancelCheckpoint) Err() error {
	if c.remaining.Add(-1) <= 0 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestBusinessBindingCancellationIsAtomic(t *testing.T) {
	for _, constrained := range []bool{false, true} {
		t.Run(strconv.FormatBool(constrained), func(t *testing.T) {
			var constraints []BusinessConstraint
			if constrained {
				constraints = []BusinessConstraint{businessFixtureConstraint()}
			}
			canceled, completed := 0, 0
			for checkpoint := int64(1); checkpoint <= 32; checkpoint++ {
				parent, cancel := context.WithCancel(context.Background())
				ctx := &businessCancelCheckpoint{Context: parent, cancel: cancel}
				ctx.remaining.Store(checkpoint)
				out, err := BindBusinessConstraints(ctx, parserBinding(), "SELECT id FROM analytics.sales ORDER BY id", nil, constraints)
				cancel()
				if err != nil {
					canceled++
					if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(out, BusinessBoundQuery{}) {
						t.Fatalf("checkpoint %d exposed partial SQL, parameters, or a receipt on cancellation: %v", checkpoint, err)
					}
				} else {
					completed++
					if out.SQL == "" || constrained && (len(out.Parameters) != 1 || len(out.Receipt.Bindings) != 1) {
						t.Fatalf("checkpoint %d returned incomplete successful binding", checkpoint)
					}
				}
			}
			if canceled < 2 || completed == 0 {
				t.Fatal("did not exercise both cancellation and completed binding")
			}
		})
	}
}
