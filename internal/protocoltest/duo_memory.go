package protocoltest

// Memory phase of the duo environment. Both instances are observed
// SEPARATELY over their /api/v1/debug endpoints, so a retention slope
// attributes directly: tb2 slope → gateway/conversion path (the #1255
// class), tb1 slope → vmodel serving path. The parent process drives
// requests but is never measured — its own allocations cannot pollute the
// numbers the way a shared-heap setup would.

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// DuoMemoryConfig parameterizes RunMemoryPhase.
type DuoMemoryConfig struct {
	Route     *DuoRoute // conversion route to drive (default DuoDefaultRoute)
	BodyBytes int       // conversation size per request (default 2MB)
	Warmup    int       // warmup requests before the baseline (default 3)
	Batch     int       // requests per sequential batch, two batches are run (default 15)
	Workers   int       // concurrent workers in the burst phase (default 4)
	PerWorker int       // requests per worker in the burst phase (default 5)
	// ReadDelay throttles client-side SSE consumption (see slowReader),
	// building real TCP backpressure against tb2. 0 = read at full speed.
	ReadDelay  time.Duration
	ProfileDir string // write pprof heap profiles here ("" = skip)
	Progress   func(format string, args ...any)
}

func (c *DuoMemoryConfig) withDefaults() {
	if c.Route == nil {
		r := DuoDefaultRoute
		c.Route = &r
	}
	if c.BodyBytes <= 0 {
		c.BodyBytes = 2 * 1024 * 1024
	}
	if c.Warmup <= 0 {
		c.Warmup = 3
	}
	if c.Batch <= 0 {
		c.Batch = 15
	}
	if c.Workers <= 0 {
		c.Workers = 4
	}
	if c.PerWorker <= 0 {
		c.PerWorker = 5
	}
	if c.Progress == nil {
		c.Progress = func(string, ...any) {}
	}
}

// DuoInstanceMemory is the memory outcome for ONE instance.
type DuoInstanceMemory struct {
	Instance           string  `json:"instance"` // "tb1" (vmodel upstream) or "tb2" (gateway)
	BaselineHeapMB     float64 `json:"baseline_heap_mb"`
	AfterBatch1MB      float64 `json:"after_batch1_delta_mb"`
	AfterBatch2MB      float64 `json:"after_batch2_delta_mb"`
	SlopeKBPerRequest  float64 `json:"retention_slope_kb_per_request"`
	ChurnMBPerRequest  float64 `json:"alloc_churn_mb_per_request"`
	PeakHeapMB         float64 `json:"concurrent_peak_heap_mb"`
	PostBurstDeltaMB   float64 `json:"post_burst_delta_mb"`
	BaselineGoroutines int     `json:"baseline_goroutines"`
	FinalGoroutines    int     `json:"final_goroutines"`
	BaselineProfile    string  `json:"baseline_profile,omitempty"`
	FinalProfile       string  `json:"final_profile,omitempty"`
}

// DuoMemoryReport is the outcome of RunMemoryPhase across both instances.
type DuoMemoryReport struct {
	Route             string            `json:"route"`
	BodyBytes         int               `json:"body_bytes"`
	Batch             int               `json:"batch_requests"` // two sequential batches of this size are run
	ReadDelayMS       int               `json:"read_delay_ms"`  // client-side slow-reader pause (0 = full speed)
	ConcurrentWorkers int               `json:"concurrent_workers"`
	ConcurrentTotal   int               `json:"concurrent_requests"`
	TB1               DuoInstanceMemory `json:"tb1"`
	TB2               DuoInstanceMemory `json:"tb2"`
}

// Instances returns both per-instance results, tb1 first.
func (r *DuoMemoryReport) Instances() []*DuoInstanceMemory {
	return []*DuoInstanceMemory{&r.TB1, &r.TB2}
}

// MaxSlopeKB returns the larger of the two instances' retention slopes.
func (r *DuoMemoryReport) MaxSlopeKB() float64 {
	return max(r.TB1.SlopeKBPerRequest, r.TB2.SlopeKBPerRequest)
}

// duoMemProbe accumulates the per-instance samples during a memory phase run.
type duoMemProbe struct {
	inst        *DuoInstance
	result      DuoInstanceMemory
	baseline    uint64
	after1      uint64
	totalAlloc0 uint64
	peak        uint64
}

// forEachProbe runs fn against both probes concurrently. The instances are
// independent processes, so their forced-GC pauses (the expensive part of a
// checkpoint) need not be paid back-to-back; the parent is never measured, so
// the extra goroutine is free.
func forEachProbe(probes []*duoMemProbe, fn func(p *duoMemProbe) error) error {
	errs := make([]error, len(probes))
	var wg sync.WaitGroup
	for i, p := range probes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = fn(p)
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

// RunMemoryPhase measures allocation churn, post-GC retention slope, and
// concurrent-burst peak heap on one conversion route — separately for tb1
// and tb2. A near-zero slope means no per-request leak on that instance
// (reference numbers live with duo_test.go's threshold).
func (env *DuoEnv) RunMemoryPhase(cfg DuoMemoryConfig) (*DuoMemoryReport, error) {
	cfg.withDefaults()
	route := *cfg.Route
	body := BuildConversationBody(route, cfg.BodyBytes, true)
	report := &DuoMemoryReport{
		Route:             route.Name,
		BodyBytes:         len(body),
		Batch:             cfg.Batch,
		ReadDelayMS:       int(cfg.ReadDelay / time.Millisecond),
		ConcurrentWorkers: cfg.Workers,
		ConcurrentTotal:   cfg.Workers * cfg.PerWorker,
	}
	probes := []*duoMemProbe{
		{inst: env.TB1, result: DuoInstanceMemory{Instance: "tb1"}},
		{inst: env.TB2, result: DuoInstanceMemory{Instance: "tb2"}},
	}

	cfg.Progress("route %s: warmup %d requests, body %.2f MB, read delay %s",
		route.Name, cfg.Warmup, float64(len(body))/1024/1024, cfg.ReadDelay)
	for i := 0; i < cfg.Warmup; i++ {
		if _, err := env.DrainStreaming(route, body, cfg.ReadDelay); err != nil {
			return nil, fmt.Errorf("warmup request %d: %w", i, err)
		}
	}

	if err := forEachProbe(probes, func(p *duoMemProbe) error {
		m, err := p.inst.MemStats(true)
		if err != nil {
			return err
		}
		p.baseline = m.HeapAllocBytes
		p.totalAlloc0 = m.TotalAllocBytes
		p.result.BaselineHeapMB = float64(m.HeapAllocBytes) / 1024 / 1024
		p.result.BaselineGoroutines = m.NumGoroutine
		if cfg.ProfileDir != "" {
			path, err := p.inst.WriteHeapProfile(cfg.ProfileDir,
				fmt.Sprintf("duo-%s-%s-baseline.pb.gz", route.Name, p.inst.Name))
			if err != nil {
				return err
			}
			p.result.BaselineProfile = path
		}
		return nil
	}); err != nil {
		return nil, err
	}

	runBatch := func() error {
		for i := 0; i < cfg.Batch; i++ {
			if _, err := env.DrainStreaming(route, body, cfg.ReadDelay); err != nil {
				return fmt.Errorf("sequential request: %w", err)
			}
		}
		return nil
	}
	cfg.Progress("route %s: sequential 2 batches × %d requests", route.Name, cfg.Batch)
	if err := runBatch(); err != nil {
		return nil, err
	}
	if err := forEachProbe(probes, func(p *duoMemProbe) error {
		m, err := p.inst.MemStats(true)
		if err != nil {
			return err
		}
		p.after1 = m.HeapAllocBytes
		p.result.AfterBatch1MB = float64(int64(m.HeapAllocBytes)-int64(p.baseline)) / 1024 / 1024
		return nil
	}); err != nil {
		return nil, err
	}
	if err := runBatch(); err != nil {
		return nil, err
	}
	if err := forEachProbe(probes, func(p *duoMemProbe) error {
		m, err := p.inst.MemStats(true)
		if err != nil {
			return err
		}
		p.result.AfterBatch2MB = float64(int64(m.HeapAllocBytes)-int64(p.baseline)) / 1024 / 1024
		p.result.SlopeKBPerRequest = (float64(int64(m.HeapAllocBytes)) - float64(int64(p.after1))) / float64(cfg.Batch) / 1024
		p.result.ChurnMBPerRequest = float64(m.TotalAllocBytes-p.totalAlloc0) / float64(2*cfg.Batch) / 1024 / 1024
		return nil
	}); err != nil {
		return nil, err
	}

	// Concurrent burst with per-instance live-heap sampling over HTTP.
	cfg.Progress("route %s: concurrent burst %d workers × %d requests", route.Name, cfg.Workers, cfg.PerWorker)
	stop := make(chan struct{})
	samplerDone := make(chan struct{})
	go func() {
		defer close(samplerDone)
		// ReadMemStats on the target is a stop-the-world operation; 100ms
		// still catches a burst peak without perturbing the workload.
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				for _, p := range probes {
					if m, err := p.inst.MemStats(false); err == nil && m.HeapAllocBytes > p.peak {
						p.peak = m.HeapAllocBytes
					}
				}
			}
		}
	}()
	var wg sync.WaitGroup
	errCh := make(chan error, cfg.Workers)
	for g := 0; g < cfg.Workers; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < cfg.PerWorker; i++ {
				if _, err := env.DrainStreaming(route, body, cfg.ReadDelay); err != nil {
					errCh <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(stop)
	<-samplerDone
	select {
	case err := <-errCh:
		return nil, fmt.Errorf("concurrent request: %w", err)
	default:
	}

	if err := forEachProbe(probes, func(p *duoMemProbe) error {
		p.result.PeakHeapMB = float64(p.peak) / 1024 / 1024
		m, err := p.inst.MemStats(true)
		if err != nil {
			return err
		}
		p.result.PostBurstDeltaMB = float64(int64(m.HeapAllocBytes)-int64(p.baseline)) / 1024 / 1024
		p.result.FinalGoroutines = m.NumGoroutine
		if cfg.ProfileDir != "" {
			path, err := p.inst.WriteHeapProfile(cfg.ProfileDir,
				fmt.Sprintf("duo-%s-%s-final.pb.gz", route.Name, p.inst.Name))
			if err != nil {
				return err
			}
			p.result.FinalProfile = path
		}
		return nil
	}); err != nil {
		return nil, err
	}

	report.TB1 = probes[0].result
	report.TB2 = probes[1].result
	return report, nil
}
