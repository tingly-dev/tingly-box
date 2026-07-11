// Package memlimit makes the Go runtime aware of the container memory limit.
//
// Inside a memory-limited cgroup (Docker/Kubernetes --memory), the Go GC has
// no idea a cap exists: with the default GOGC=100 the heap is allowed to grow
// to roughly twice the live set before a collection is forced, and freed spans
// are returned to the OS lazily. Under bursty allocation (large request bodies,
// protocol conversion, tokenization) RSS can shoot past the cgroup limit and
// the kernel OOM-kills the process with SIGKILL — no panic, no stack trace,
// exit code 0 (#1255).
//
// Setting the runtime soft memory limit (GOMEMLIMIT) slightly below the cgroup
// limit turns that hard kill into GC back-pressure: the collector works harder
// as the limit approaches instead of letting the kernel shoot the process.
package memlimit

import (
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

// defaultCgroupFiles are the standard in-container locations of the memory
// limit, cgroup v2 first. Kept as a var for tests.
var defaultCgroupFiles = []string{
	"/sys/fs/cgroup/memory.max",                  // cgroup v2
	"/sys/fs/cgroup/memory/memory.limit_in_bytes", // cgroup v1
}

// ratio of the cgroup limit to hand to the runtime. The reserve headroom
// covers non-Go memory the runtime cannot track (CGo, mmap'd files, goroutine
// stacks beyond the heap accounting, kernel buffers billed to the cgroup).
const ratioNum, ratioDen = 9, 10

// noLimitThreshold: cgroup v1 reports "unlimited" as a huge page-rounded
// value (~2^63); treat anything above 2^62 as no limit.
const noLimitThreshold = int64(1) << 62

// SetFromCgroup sets the Go runtime soft memory limit to 90% of the cgroup
// memory limit when the process runs inside a memory-limited cgroup.
//
// It is a no-op when the GOMEMLIMIT environment variable is set (the runtime
// already honors it and the operator's choice wins), when no cgroup limit
// file is readable (non-Linux, or bare metal), or when the cgroup reports no
// limit. Returns the limit that was applied and whether one was applied.
func SetFromCgroup() (applied int64, ok bool) {
	if os.Getenv("GOMEMLIMIT") != "" {
		return 0, false
	}
	limit, ok := readCgroupLimit(defaultCgroupFiles)
	if !ok {
		return 0, false
	}
	soft := limit / ratioDen * ratioNum
	if soft <= 0 {
		return 0, false
	}
	debug.SetMemoryLimit(soft)
	return soft, true
}

// readCgroupLimit returns the first parseable memory limit from files.
func readCgroupLimit(files []string) (int64, bool) {
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if limit, ok := parseLimit(string(data)); ok {
			return limit, true
		}
	}
	return 0, false
}

// parseLimit parses a cgroup memory limit value. "max" (v2) and absurdly
// large values (v1 unlimited) mean no limit.
func parseLimit(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "max" {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v <= 0 || v >= noLimitThreshold {
		return 0, false
	}
	return v, true
}
