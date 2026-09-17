package runner

import (
	"context"
	"sync"
)

type Task[T any] struct {
	Name string
	Run  func(ctx context.Context) T
}

func Run[T any](ctx context.Context, tasks []Task[T], parallel int, onResult func(idx int, res T)) []T {
	if parallel < 1 {
		parallel = 1
	}
	results := make([]T, len(tasks))
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	var cbMu sync.Mutex
	for i, t := range tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res := t.Run(ctx)
			results[i] = res
			if onResult != nil {
				cbMu.Lock()
				onResult(i, res)
				cbMu.Unlock()
			}
		}()
	}
	wg.Wait()
	return results
}
