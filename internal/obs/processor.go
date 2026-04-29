package obs

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// RecordProcessor is the OTel-shaped hot-path interface.
// Emit must be non-blocking; callers on the request goroutine must not be delayed.
type RecordProcessor interface {
	Emit(r *Record)
	ForceFlush(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

// RecordExporter is called by BatchProcessor with a slice of Records to persist.
type RecordExporter interface {
	Export(ctx context.Context, records []*Record) error
	Shutdown(ctx context.Context) error
}

// BatchProcessor batches Records and forwards them to a RecordExporter on a
// periodic ticker or when the batch reaches maxBatch size.
//
// Emit is non-blocking: if the internal queue is full the record is dropped
// and a counter is incremented.
type BatchProcessor struct {
	queue    chan *Record
	exporter RecordExporter
	ticker   *time.Ticker
	flushCh  chan chan error // synchronous flush requests
	done     chan struct{}
	wg       sync.WaitGroup
	dropped  atomic.Int64

	maxBatch int
}

// BatchProcessorOptions configures a BatchProcessor.
type BatchProcessorOptions struct {
	// QueueSize is the capacity of the internal channel (default 1024).
	QueueSize int
	// MaxBatch is the maximum number of records per Export call (default 256).
	MaxBatch int
	// FlushInterval is how often the worker drains the queue (default 5s).
	FlushInterval time.Duration
}

func (o *BatchProcessorOptions) withDefaults() {
	if o.QueueSize <= 0 {
		o.QueueSize = 1024
	}
	if o.MaxBatch <= 0 {
		o.MaxBatch = 256
	}
	if o.FlushInterval <= 0 {
		o.FlushInterval = 5 * time.Second
	}
}

// NewBatchProcessor creates a BatchProcessor and starts its background worker.
func NewBatchProcessor(exporter RecordExporter, opts BatchProcessorOptions) *BatchProcessor {
	opts.withDefaults()
	bp := &BatchProcessor{
		queue:    make(chan *Record, opts.QueueSize),
		exporter: exporter,
		ticker:   time.NewTicker(opts.FlushInterval),
		flushCh:  make(chan chan error, 4),
		done:     make(chan struct{}),
		maxBatch: opts.MaxBatch,
	}
	bp.wg.Add(1)
	go bp.worker()
	return bp
}

// Emit enqueues r for export. Drops the record (incrementing the dropped counter)
// if the internal queue is full.
func (bp *BatchProcessor) Emit(r *Record) {
	select {
	case bp.queue <- r:
	default:
		bp.dropped.Add(1)
	}
}

// ForceFlush triggers an immediate export of any pending records and waits for it.
func (bp *BatchProcessor) ForceFlush(ctx context.Context) error {
	errCh := make(chan error, 1)
	select {
	case bp.flushCh <- errCh:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Shutdown stops the ticker, drains the queue, exports remaining records, then
// shuts down the exporter.
func (bp *BatchProcessor) Shutdown(ctx context.Context) error {
	bp.ticker.Stop()
	close(bp.done)
	// Wait for the worker to exit, but respect ctx deadline.
	workerDone := make(chan struct{})
	go func() { bp.wg.Wait(); close(workerDone) }()
	select {
	case <-workerDone:
	case <-ctx.Done():
		return ctx.Err()
	}
	n := bp.dropped.Load()
	if n > 0 {
		logrus.Warnf("obs: BatchProcessor dropped %d records (queue was full)", n)
	}
	return bp.exporter.Shutdown(ctx)
}

func (bp *BatchProcessor) worker() {
	defer bp.wg.Done()

	var pending []*Record

	flush := func() {
		if len(pending) == 0 {
			return
		}
		if err := bp.exporter.Export(context.Background(), pending); err != nil {
			logrus.Warnf("obs: export error: %v", err)
		}
		pending = pending[:0]
	}

	for {
		select {
		case r := <-bp.queue:
			pending = append(pending, r)
			if len(pending) >= bp.maxBatch {
				flush()
			}

		case <-bp.ticker.C:
			flush()

		case errCh := <-bp.flushCh:
			// Drain any queued records first.
		drain:
			for {
				select {
				case r := <-bp.queue:
					pending = append(pending, r)
				default:
					break drain
				}
			}
			flush()
			errCh <- nil

		case <-bp.done:
			// Final drain.
		finalDrain:
			for {
				select {
				case r := <-bp.queue:
					pending = append(pending, r)
				default:
					break finalDrain
				}
			}
			flush()
			return
		}
	}
}
