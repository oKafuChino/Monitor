package metric

import (
 "context"
 "fmt"
 "sync"
)

type readBudgetKey struct{}
type readBudget struct { mu sync.Mutex; points, series int }

// WithReadBudget limits work before query data is materialized. Background
// compaction has no budget, while every external RPC query shares one budget.
func WithReadBudget(ctx context.Context, points, series int) context.Context {
 return context.WithValue(ctx, readBudgetKey{}, &readBudget{points:points, series:series})
}
func consumeReadBudget(ctx context.Context, points, series int) error {
 if err := ctx.Err(); err != nil { return err }
 budget, _ := ctx.Value(readBudgetKey{}).(*readBudget)
 if budget == nil { return nil }
 budget.mu.Lock(); defer budget.mu.Unlock()
 if points > budget.points || series > budget.series { return fmt.Errorf("metric query budget exceeded; reduce nodes, metrics or time range") }
 budget.points -= points; budget.series -= series
 return nil
}
func checkReadAllocation(ctx context.Context, points int) error {
 budget, _ := ctx.Value(readBudgetKey{}).(*readBudget)
 if budget == nil { return ctx.Err() }
 budget.mu.Lock(); defer budget.mu.Unlock()
 if points > budget.points { return fmt.Errorf("metric query point budget exceeded") }
 return ctx.Err()
}
