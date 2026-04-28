package task

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	schedulerDefaultPoll  = 5 * time.Second
	schedulerBatchSize    = 50
	schedulerWaitInterval = 500 * time.Millisecond // Wait() poll interval
)

type scheduler struct {
	store    Store
	dispatch func(ctx context.Context, t *Task)
	poll     time.Duration
}

func newScheduler(store Store, dispatch func(ctx context.Context, t *Task)) *scheduler {
	return &scheduler{store: store, dispatch: dispatch, poll: schedulerDefaultPoll}
}

// run is the scheduler goroutine. It ticks once immediately then on every
// poll interval until ctx is cancelled.
func (s *scheduler) run(ctx context.Context) {
	ticker := time.NewTicker(s.poll)
	defer ticker.Stop()
	s.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *scheduler) tick(ctx context.Context) {
	tasks, err := s.store.FindDueTasks(ctx, time.Now(), schedulerBatchSize)
	if err != nil {
		logrus.WithError(err).Warn("task scheduler: FindDueTasks failed")
		return
	}
	for i := range tasks {
		s.dispatch(ctx, &tasks[i])
	}
}
