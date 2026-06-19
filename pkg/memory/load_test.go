package memory_test

import (
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/pkg/memory"
)

// TestConcurrentMemoryLeak simulates real concurrent API load
func TestConcurrentMemoryLeak(t *testing.T) {
	// Simulate real API request body (larger than test examples)
	largeRequestBody := generateLargeRequestBody(50, 500) // 50 messages, 500 chars each

	t.Run("ConcurrentWithoutPooling", func(t *testing.T) {
		testConcurrentWithoutPooling(t, largeRequestBody)
	})

	t.Run("ConcurrentWithPooling", func(t *testing.T) {
		testConcurrentWithPooling(t, largeRequestBody)
	})
}

func testConcurrentWithoutPooling(t *testing.T, requestBody string) {
	runtime.GC()
	time.Sleep(100 * time.Millisecond) // Let GC settle

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	// Simulate 100 concurrent requests
	workers := 100
	requestsPerWorker := 100

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerWorker; j++ {
				bodyBytes := []byte(requestBody)
				var req protocol.AnthropicBetaMessagesRequest
				if err := json.Unmarshal(bodyBytes, &req); err != nil {
					t.Errorf("Failed to unmarshal: %v", err)
				}
			}
		}()
	}
	wg.Wait()

	// Force GC and measure
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.ReadMemStats(&after)

	allocatedMB := float64(after.TotalAlloc-before.TotalAlloc) / 1024 / 1024
	heapMB := float64(after.HeapInuse) / 1024 / 1024
	objects := after.HeapObjects - before.HeapObjects

	t.Logf("Concurrent Without Pooling (%d workers × %d requests = %d total):",
		workers, requestsPerWorker, workers*requestsPerWorker)
	t.Logf("  Allocated: %.2f MB", allocatedMB)
	t.Logf("  Heap Inuse: %.2f MB", heapMB)
	t.Logf("  Heap Objects: %d", objects)
	t.Logf("  Per-request overhead: %.2f KB", (allocatedMB*1024)/float64(workers*requestsPerWorker))
}

func testConcurrentWithPooling(t *testing.T, requestBody string) {
	runtime.GC()
	time.Sleep(100 * time.Millisecond) // Let GC settle

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	// Simulate 100 concurrent requests
	workers := 100
	requestsPerWorker := 100

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerWorker; j++ {
				bodyBytes := []byte(requestBody)
				bodyCopy := memory.CopyRequestBody(bodyBytes)
				var req protocol.AnthropicBetaMessagesRequest
				if err := json.Unmarshal(bodyCopy, &req); err != nil {
					t.Errorf("Failed to unmarshal: %v", err)
				}
			}
		}()
	}
	wg.Wait()

	// Force GC and measure
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.ReadMemStats(&after)

	allocatedMB := float64(after.TotalAlloc-before.TotalAlloc) / 1024 / 1024
	heapMB := float64(after.HeapInuse) / 1024 / 1024
	objects := after.HeapObjects - before.HeapObjects

	t.Logf("Concurrent With Pooling (%d workers × %d requests = %d total):",
		workers, requestsPerWorker, workers*requestsPerWorker)
	t.Logf("  Allocated: %.2f MB", allocatedMB)
	t.Logf("  Heap Inuse: %.2f MB", heapMB)
	t.Logf("  Heap Objects: %d", objects)
	t.Logf("  Per-request overhead: %.2f KB", (allocatedMB*1024)/float64(workers*requestsPerWorker))
}

// TestSustainedLoad simulates sustained load over time
func TestSustainedLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping sustained load test in short mode")
	}

	largeRequestBody := generateLargeRequestBody(20, 200)

	t.Run("SustainedWithoutPooling", func(t *testing.T) {
		testSustainedLoad(t, largeRequestBody, false)
	})

	t.Run("SustainedWithPooling", func(t *testing.T) {
		testSustainedLoad(t, largeRequestBody, true)
	})
}

func testSustainedLoad(t *testing.T, requestBody string, withPooling bool) {
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	var memStats []runtime.MemStats
	sampleInterval := 500 * time.Millisecond

	workers := 10
	requestsPerWorker := 50

	// Sample memory during load
	ticker := time.NewTicker(sampleInterval)
	defer ticker.Stop()

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Memory sampler
	go func() {
		for {
			select {
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				memStats = append(memStats, m)
			case <-stop:
				return
			}
		}
	}()

	// Load generator
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerWorker; j++ {
				bodyBytes := []byte(requestBody)
				var req protocol.AnthropicBetaMessagesRequest
				var err error

				if withPooling {
					bodyCopy := memory.CopyRequestBody(bodyBytes)
					err = json.Unmarshal(bodyCopy, &req)
				} else {
					err = json.Unmarshal(bodyBytes, &req)
				}

				if err != nil {
					t.Errorf("Failed to unmarshal: %v", err)
				}
			}
		}()
	}
	wg.Wait()
	close(stop)

	// Final GC and measurement
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	var final runtime.MemStats
	runtime.ReadMemStats(&final)

	poolingLabel := "Without"
	if withPooling {
		poolingLabel = "With"
	}

	t.Logf("%s Pooling - Sustained Load (%d workers × %d requests):",
		poolingLabel, workers, requestsPerWorker)
	t.Logf("  Final Heap Inuse: %.2f MB", float64(final.HeapInuse)/1024/1024)
	t.Logf("  Final Heap Objects: %d", final.HeapObjects)

	if len(memStats) > 1 {
		initialHeap := float64(memStats[0].HeapInuse) / 1024 / 1024
		finalHeap := float64(memStats[len(memStats)-1].HeapInuse) / 1024 / 1024
		growth := finalHeap - initialHeap
		t.Logf("  Heap Growth: %.2f MB → %.2f MB (%.2f MB growth)",
			initialHeap, finalHeap, growth)

		if growth > 50 {
			t.Logf("  ⚠️  WARNING: Significant heap growth suggests memory leak")
		} else {
			t.Logf("  ✅ Heap growth is acceptable")
		}
	}
}

// TestLargeScaleMemoryLeak tests large-scale request processing
func TestLargeScaleMemoryLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large-scale test in short mode")
	}

	requestBody := generateLargeRequestBody(10, 200)

	t.Run("LargeScaleWithoutPooling", func(t *testing.T) {
		testLargeScaleMemoryLeak(t, requestBody, false)
	})

	t.Run("LargeScaleWithPooling", func(t *testing.T) {
		testLargeScaleMemoryLeak(t, requestBody, true)
	})
}

func testLargeScaleMemoryLeak(t *testing.T, requestBody string, withPooling bool) {
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	// Process many requests
	iterations := 100000
	for i := 0; i < iterations; i++ {
		bodyBytes := []byte(requestBody)
		var req protocol.AnthropicBetaMessagesRequest
		var err error

		if withPooling {
			bodyCopy := memory.CopyRequestBody(bodyBytes)
			err = json.Unmarshal(bodyCopy, &req)
		} else {
			err = json.Unmarshal(bodyBytes, &req)
		}

		if err != nil {
			t.Errorf("Failed to unmarshal: %v", err)
		}

		// Periodic GC to see accumulation pattern
		if i%10000 == 9999 {
			runtime.GC()
		}
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.ReadMemStats(&after)

	allocatedMB := float64(after.TotalAlloc-before.TotalAlloc) / 1024 / 1024
	heapMB := float64(after.HeapInuse) / 1024 / 1024
	objects := after.HeapObjects - before.HeapObjects

	poolingLabel := "Without"
	if withPooling {
		poolingLabel = "With"
	}

	t.Logf("%s Pooling - Large Scale (%d iterations):", poolingLabel, iterations)
	t.Logf("  Total Allocated: %.2f MB", allocatedMB)
	t.Logf("  Heap Inuse: %.2f MB", heapMB)
	t.Logf("  Heap Objects: %d", objects)
	t.Logf("  Per-request overhead: %.2f KB", (allocatedMB*1024)/float64(iterations))

	if heapMB > 100 {
		t.Logf("  ⚠️  WARNING: High heap usage (%.2f MB)", heapMB)
	} else {
		t.Logf("  ✅ Heap usage is acceptable")
	}
}

// TestMemoryStability tests memory stability over extended operations
func TestMemoryStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stability test in short mode")
	}

	requestBody := generateLargeRequestBody(5, 100)

	t.Run("StabilityWithoutPooling", func(t *testing.T) {
		testMemoryStability(t, requestBody, false)
	})

	t.Run("StabilityWithPooling", func(t *testing.T) {
		testMemoryStability(t, requestBody, true)
	})
}

func testMemoryStability(t *testing.T, requestBody string, withPooling bool) {
	var memReadings []float64
	sampleInterval := 1000
	iterations := 10000

	for i := 0; i < iterations; i++ {
		bodyBytes := []byte(requestBody)
		var req protocol.AnthropicBetaMessagesRequest
		var err error

		if withPooling {
			bodyCopy := memory.CopyRequestBody(bodyBytes)
			err = json.Unmarshal(bodyCopy, &req)
		} else {
			err = json.Unmarshal(bodyBytes, &req)
		}

		if err != nil {
			t.Errorf("Failed to unmarshal: %v", err)
		}

		// Sample memory periodically
		if i%sampleInterval == 0 {
			runtime.GC()
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			heapMB := float64(m.HeapInuse) / 1024 / 1024
			memReadings = append(memReadings, heapMB)
		}
	}

	// Final reading
	runtime.GC()
	var final runtime.MemStats
	runtime.ReadMemStats(&final)
	finalHeap := float64(final.HeapInuse) / 1024 / 1024

	poolingLabel := "Without"
	if withPooling {
		poolingLabel = "With"
	}

	t.Logf("%s Pooling - Memory Stability (%d iterations):", poolingLabel, iterations)
	t.Logf("  Final Heap: %.2f MB", finalHeap)
	t.Logf("  Readings: %d samples", len(memReadings))

	if len(memReadings) >= 2 {
		min, max := memReadings[0], memReadings[0]
		for _, v := range memReadings {
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
		}
		variance := max - min
		t.Logf("  Heap range: %.2f MB - %.2f MB (variance: %.2f MB)", min, max, variance)

		if variance > 50 {
			t.Logf("  ⚠️  WARNING: High memory variance suggests instability")
		} else {
			t.Logf("  ✅ Memory usage is stable")
		}
	}
}

// Helper function to generate large request body
func generateLargeRequestBody(messageCount int, messageSize int) string {
	messages := make([]map[string]string, messageCount)
	for i := range messages {
		content := fmt.Sprintf("Message %d with padding to reach target size: %s",
			i, fmt.Sprintf("%*s", messageSize, "x"))
		messages[i] = map[string]string{
			"role":    "user",
			"content": content,
		}
	}

	// Use json.Marshal directly to avoid duplicate function
	msgBytes, _ := json.Marshal(messages)
	return fmt.Sprintf(`{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 4096,
		"messages": %s,
		"stream": true
	}`, string(msgBytes))
}
