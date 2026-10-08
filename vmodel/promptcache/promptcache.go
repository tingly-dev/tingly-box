// Package promptcache simulates an upstream provider's prompt (prefix) cache,
// so the cache hit rate a gateway setup would get can be measured without a
// real provider.
//
// Every provider prompt cache keys on the request prefix: two requests share a
// cache entry up to the first block that differs. The simulator reproduces
// exactly that and nothing provider-specific — it is an *ideal* provider:
//
//   - granularity is one prompt block (a tool definition, a system block, a
//     message content block / chat message / Responses input item), not a
//     vendor's 64- or 128-token chunk;
//   - there is no minimum cacheable length and no 20-block lookback limit;
//   - entries live for a fixed TTL (default one hour) regardless of the
//     cache_control ttl the request asked for.
//
// So the hit rate it reports is an upper bound set by prefix stability alone:
// when it is low, the requests themselves stopped sharing a prefix (client or
// gateway rewrote history); when it is high but a real provider's is low, the
// cause is the provider or the routing in front of it.
//
// Two cache disciplines are modelled, because they fail differently:
//
//   - Explicit (Anthropic Messages): only prefixes ending at a cache_control
//     breakpoint are written, and a read needs a breakpoint at or after the
//     matched prefix. Dropping cache_control on the way upstream therefore
//     shows up as zero hits, as it would on Anthropic.
//   - Automatic (OpenAI Chat / Responses and the OpenAI-compatible vendors):
//     every prefix is written and any prefix can be read.
//
// See .design/vmodel-prompt-cache.md.
package promptcache

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"sync"
	"time"
)

// Discipline selects how a request's blocks may be written to and read from
// the cache.
type Discipline int

const (
	// Automatic caches every prefix (OpenAI-style).
	Automatic Discipline = iota
	// Explicit caches only prefixes that end at a breakpoint (Anthropic-style).
	Explicit
)

// Block is one cacheable unit of a prompt, in prompt order.
type Block struct {
	// Label locates the block in the request, e.g. "messages[3].content[1]".
	Label string
	// Tokens is the block's estimated token count.
	Tokens int64
	// Breakpoint reports whether the client marked a cache breakpoint on it.
	Breakpoint bool
	// key is the canonical serialization, prompt-cache hints removed.
	key []byte
}

// Result is the outcome of one simulated request.
type Result struct {
	// PromptTokens is the whole prompt: cached + written + plain input.
	PromptTokens int64
	// CachedTokens is the prefix served from the cache.
	CachedTokens int64
	// WriteTokens is what this request newly wrote (Explicit discipline only;
	// automatic caches write for free and report nothing).
	WriteTokens int64
	// Blocks is the number of blocks in the prompt; CachedBlocks of them, in
	// order from the start, were served from the cache.
	Blocks, CachedBlocks int
	// FirstUncached labels the first block not served from the cache, or ""
	// when the whole prompt was.
	FirstUncached string
}

// UncachedInputTokens is the prompt minus cache reads and writes — Anthropic's
// wire input_tokens.
func (r Result) UncachedInputTokens() int64 {
	return r.PromptTokens - r.CachedTokens - r.WriteTokens
}

// HitRate is CachedTokens / PromptTokens, the same ratio the usage dashboard
// reports.
func (r Result) HitRate() float64 {
	if r.PromptTokens == 0 {
		return 0
	}
	return float64(r.CachedTokens) / float64(r.PromptTokens)
}

// Report is the human-readable summary the virtual model replies with.
func (r Result) Report() string {
	s := fmt.Sprintf("Prompt cache simulation: %d of %d prompt tokens read from cache (%.1f%%)",
		r.CachedTokens, r.PromptTokens, r.HitRate()*100)
	if r.WriteTokens > 0 {
		s += fmt.Sprintf(", %d written", r.WriteTokens)
	}
	if r.FirstUncached != "" {
		s += fmt.Sprintf(". Cached prefix: %d of %d blocks; first uncached block: %s", r.CachedBlocks, r.Blocks, r.FirstUncached)
	}
	return s + "."
}

const (
	// DefaultTTL is how long a written prefix stays readable.
	DefaultTTL = time.Hour
	// DefaultMaxEntries bounds memory: one entry is one cached prefix
	// (a hash and an expiry), so this is a few MB at most.
	DefaultMaxEntries = 200_000
)

// Simulator is a thread-safe prefix cache. The zero value is not usable; use
// New.
type Simulator struct {
	mu         sync.Mutex
	ttl        time.Duration
	maxEntries int
	now        func() time.Time
	entries    map[uint64]time.Time // prefix chain hash → expiry
}

// New returns a Simulator with DefaultTTL and DefaultMaxEntries.
func New() *Simulator {
	return &Simulator{
		ttl:        DefaultTTL,
		maxEntries: DefaultMaxEntries,
		now:        time.Now,
		entries:    map[uint64]time.Time{},
	}
}

// Observe simulates one request: it reads the longest cached prefix of blocks,
// then writes this request's prefixes. scope partitions the cache (one
// endpoint + model is one provider cache).
func (s *Simulator) Observe(scope string, d Discipline, blocks []Block) Result {
	res := Result{Blocks: len(blocks)}

	// chain[i] identifies the prefix blocks[0..i]: a prefix cache matches a
	// block only together with everything before it.
	chain := make([]uint64, len(blocks))
	prev := scopeSeed(scope)
	lastBreakpoint := -1
	for i, b := range blocks {
		prev = chainHash(prev, b.key)
		chain[i] = prev
		res.PromptTokens += b.Tokens
		if b.Breakpoint {
			lastBreakpoint = i
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()

	// Read: the longest stored prefix. Explicit caching can only read up to a
	// breakpoint in this request.
	readLimit := len(blocks) - 1
	if d == Explicit {
		readLimit = lastBreakpoint
	}
	matched := -1
	for i := readLimit; i >= 0; i-- {
		if exp, ok := s.entries[chain[i]]; ok && now.Before(exp) {
			matched = i
			break
		}
	}
	res.CachedBlocks = matched + 1
	for i := 0; i <= matched; i++ {
		res.CachedTokens += blocks[i].Tokens
	}
	if res.CachedBlocks < len(blocks) {
		res.FirstUncached = blocks[res.CachedBlocks].Label
	}

	// Write: every prefix (automatic) or every prefix ending at a breakpoint
	// (explicit). A hit refreshes the entry's TTL, as real caches do.
	s.evictLocked(now)
	exp := now.Add(s.ttl)
	for i := range blocks {
		if d == Explicit && !blocks[i].Breakpoint {
			continue
		}
		s.entries[chain[i]] = exp
	}
	if d == Explicit && lastBreakpoint > matched {
		for i := matched + 1; i <= lastBreakpoint; i++ {
			res.WriteTokens += blocks[i].Tokens
		}
	}
	return res
}

// evictLocked drops expired entries once the cache is full, and everything if
// that is not enough — a diagnostic cache may forget, it must not grow without
// bound.
func (s *Simulator) evictLocked(now time.Time) {
	if len(s.entries) < s.maxEntries {
		return
	}
	for k, exp := range s.entries {
		if !now.Before(exp) {
			delete(s.entries, k)
		}
	}
	if len(s.entries) >= s.maxEntries {
		s.entries = map[uint64]time.Time{}
	}
}

func scopeSeed(scope string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(scope))
	return h.Sum64()
}

func chainHash(prev uint64, key []byte) uint64 {
	h := fnv.New64a()
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], prev)
	h.Write(buf[:])
	h.Write(key)
	return h.Sum64()
}
