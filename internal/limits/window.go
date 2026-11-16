package limits

import "time"

// window is a sliding usage window made of fixed-size buckets in a ring. It
// avoids the burst-at-rollover of a tumbling window.
type window struct {
	length time.Duration
	bucket time.Duration
	n      int
	reqs   []int64
	toks   []int64
	head   int64 // absolute index of the newest bucket seen
	init   bool
}

func newWindow(length time.Duration) *window {
	bucket := length / maxWindowBuckets
	if bucket < minWindowBucketSize {
		bucket = minWindowBucketSize
	}
	n := int((length + bucket - 1) / bucket)
	if n < 1 {
		n = 1
	}
	return &window{length: length, bucket: bucket, n: n, reqs: make([]int64, n), toks: make([]int64, n)}
}

func (w *window) idx(t time.Time) int64 { return t.UnixNano() / int64(w.bucket) }

// advance rotates the ring so head == idx(now), clearing expired buckets.
func (w *window) advance(now time.Time) {
	cur := w.idx(now)
	if !w.init {
		w.init, w.head = true, cur
		return
	}
	if cur <= w.head {
		return
	}
	gap := cur - w.head
	if gap >= int64(w.n) {
		for i := range w.reqs {
			w.reqs[i], w.toks[i] = 0, 0
		}
	} else {
		for i := int64(1); i <= gap; i++ {
			s := int((w.head + i) % int64(w.n))
			w.reqs[s], w.toks[s] = 0, 0
		}
	}
	w.head = cur
}

// add accounts usage at time at. Samples older than the window are dropped.
func (w *window) add(at time.Time, reqs, toks int64) {
	w.advance(at)
	i := w.idx(at)
	if w.head-i >= int64(w.n) {
		return
	}
	s := int(((i % int64(w.n)) + int64(w.n)) % int64(w.n))
	w.reqs[s] += reqs
	w.toks[s] += toks
}

func (w *window) sums(now time.Time) (reqs, toks int64) {
	w.advance(now)
	for i := range w.reqs {
		reqs += w.reqs[i]
		toks += w.toks[i]
	}
	return
}

func (w *window) empty(now time.Time) bool {
	r, t := w.sums(now)
	return r == 0 && t == 0
}

// freeIn estimates how long until the oldest non-empty bucket leaves the window.
func (w *window) freeIn(now time.Time) time.Duration {
	w.advance(now)
	for age := int64(w.n - 1); age >= 0; age-- {
		abs := w.head - age
		s := int(((abs % int64(w.n)) + int64(w.n)) % int64(w.n))
		if w.reqs[s] != 0 || w.toks[s] != 0 {
			expiry := time.Unix(0, (abs+int64(w.n))*int64(w.bucket))
			d := expiry.Sub(now)
			if d < time.Second {
				d = time.Second
			}
			return d
		}
	}
	return time.Second
}
