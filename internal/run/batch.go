package run

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	api2convert "github.com/QaamGo/api2convert-go"

	"github.com/QaamGo/api2convert-cli/internal/ui"
)

// Item is one unit of batch work: an input and the target to convert it to.
type Item struct {
	Input  string
	Target string
}

// FileError pairs a failed input with its error.
type FileError struct {
	Input string
	Err   error
}

// Summary aggregates the outcome of a batch.
type Summary struct {
	Results []Result
	Errors  []FileError
}

// Total returns the number of inputs processed.
func (s Summary) Total() int { return len(s.Results) + len(s.Errors) }

// Batch converts many inputs to a single shared target.
func Batch(ctx context.Context, c *api2convert.Client, inputs []string, target, out string, conc int, o Options, overall ui.Progress, failFast bool) Summary {
	items := make([]Item, len(inputs))
	for i, in := range inputs {
		items[i] = Item{Input: in, Target: target}
	}
	return BatchItems(ctx, c, items, out, conc, o, overall, failFast)
}

// BatchItems converts a heterogeneous set of items (each with its own target)
// concurrently (bounded by conc, default NumCPU). It is fail-soft unless
// failFast is set, which cancels the context on the first error.
func BatchItems(ctx context.Context, c *api2convert.Client, items []Item, out string, conc int, o Options, overall ui.Progress, failFast bool) Summary {
	if conc <= 0 {
		conc = runtime.NumCPU()
	}
	if conc > len(items) {
		conc = len(items)
	}

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		sum  Summary
		done int
	)
	silent := silentProgress()
	sem := make(chan struct{}, conc)

	overall.Start(fmt.Sprintf("Converting 0/%d", len(items)))
	for _, it := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(it Item) {
			defer wg.Done()
			defer func() { <-sem }()

			res, err := ConvertOne(cctx, c, it.Input, it.Target, out, o, silent)

			mu.Lock()
			done++
			if err != nil {
				sum.Errors = append(sum.Errors, FileError{Input: it.Input, Err: err})
				if failFast {
					cancel()
				}
			} else {
				sum.Results = append(sum.Results, res)
			}
			overall.Update(fmt.Sprintf("Converting %d/%d", done, len(items)))
			mu.Unlock()
		}(it)
	}
	wg.Wait()
	overall.Stop()
	return sum
}
