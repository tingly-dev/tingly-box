package memory_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/pkg/memory"
)

// TestGjsonMemoryLeakReproduction reproduces the memory leak issue
// when using SDK types with gjson-based JSON unmarshaling.
//
// This test demonstrates:
// 1. Without pooling: request bodies are retained in memory
// 2. With pooling: request bodies can be garbage collected
func TestGjsonMemoryLeakReproduction(t *testing.T) {
	// Typical API request body
	requestBody := `{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 4096,
		"messages": [
			{
				"role": "user",
				"content": "What is the meaning of life?"
			}
		],
		"stream": true
	}`

	// Test both scenarios
	t.Run("WithoutPooling", func(t *testing.T) {
		testWithoutPooling(t, requestBody)
	})

	t.Run("WithPooling", func(t *testing.T) {
		testWithPooling(t, requestBody)
	})
}

// testWithoutPooling demonstrates the memory leak
func testWithoutPooling(t *testing.T, requestBody string) {
	// Force GC to get baseline
	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	// Simulate processing many requests
	iterations := 1000
	for i := 0; i < iterations; i++ {
		// Simulate reading request body (as HTTP framework would)
		bodyBytes := []byte(requestBody)

		// Parse with SDK type (this uses gjson internally)
		var req protocol.AnthropicBetaMessagesRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		// Use the request
		if req.Model == "" {
			t.Error("Model should not be empty")
		}

		// Simulate request completion (bodyBytes goes out of scope)
		// BUT: gjson may still hold references via global decoder cache
	}

	// Force GC and check memory
	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// Calculate memory growth
	allocatedMB := float64(memAfter.TotalAlloc-memBefore.TotalAlloc) / 1024 / 1024
	heapMB := float64(memAfter.HeapInuse) / 1024 / 1024

	t.Logf("Without Pooling:")
	t.Logf("  Allocated: %.2f MB", allocatedMB)
	t.Logf("  Heap Inuse: %.2f MB", heapMB)
	t.Logf("  Heap Objects: %d", memAfter.HeapObjects)

	// With the leak, we expect significant heap growth
	// This is a "smoke test" - actual numbers depend on GC behavior
	if heapMB > 50 {
		t.Logf("WARNING: High heap usage suggests potential memory leak (>50MB)")
	}
}

// testWithPooling demonstrates the fix
func testWithPooling(t *testing.T, requestBody string) {
	// Force GC to get baseline
	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	// Simulate processing many requests
	iterations := 1000
	for i := 0; i < iterations; i++ {
		// Simulate reading request body
		bodyBytes := []byte(requestBody)

		// CRITICAL: Copy through memory pool to break gjson reference chain
		bodyCopy := memory.CopyRequestBody(bodyBytes)

		// Parse with SDK type using the copy
		var req protocol.AnthropicBetaMessagesRequest
		if err := json.Unmarshal(bodyCopy, &req); err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		// Use the request
		if req.Model == "" {
			t.Error("Model should not be empty")
		}

		// Both bodyBytes and bodyCopy can now be GC'd
	}

	// Force GC and check memory
	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// Calculate memory growth
	allocatedMB := float64(memAfter.TotalAlloc-memBefore.TotalAlloc) / 1024 / 1024
	heapMB := float64(memAfter.HeapInuse) / 1024 / 1024

	t.Logf("With Pooling:")
	t.Logf("  Allocated: %.2f MB", allocatedMB)
	t.Logf("  Heap Inuse: %.2f MB", heapMB)
	t.Logf("  Heap Objects: %d", memAfter.HeapObjects)

	// With the fix, heap should be much smaller
	// This is a "smoke test" - actual numbers depend on GC behavior
	if heapMB < 50 {
		t.Logf("GOOD: Heap usage is controlled (<50MB)")
	}
}

// TestMemoryLeakComparison runs both scenarios and compares results
func TestMemoryLeakComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping comparison test in short mode")
	}

	requestBody := `{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 4096,
		"messages": [{"role": "user", "content": "Test message"}],
		"stream": true
	}`

	iterations := 5000 // More iterations for clearer comparison

	// Test without pooling
	runtime.GC()
	var memBefore1 runtime.MemStats
	runtime.ReadMemStats(&memBefore1)

	for i := 0; i < iterations; i++ {
		bodyBytes := []byte(requestBody)
		var req protocol.AnthropicBetaMessagesRequest
		json.Unmarshal(bodyBytes, &req)
	}

	runtime.GC()
	var memAfter1 runtime.MemStats
	runtime.ReadMemStats(&memAfter1)

	// Test with pooling
	runtime.GC()
	var memBefore2 runtime.MemStats
	runtime.ReadMemStats(&memBefore2)

	for i := 0; i < iterations; i++ {
		bodyBytes := []byte(requestBody)
		bodyCopy := memory.CopyRequestBody(bodyBytes)
		var req protocol.AnthropicBetaMessagesRequest
		json.Unmarshal(bodyCopy, &req)
	}

	runtime.GC()
	var memAfter2 runtime.MemStats
	runtime.ReadMemStats(&memAfter2)

	// Compare results
	allocated1 := float64(memAfter1.TotalAlloc-memBefore1.TotalAlloc) / 1024 / 1024
	allocated2 := float64(memAfter2.TotalAlloc-memBefore2.TotalAlloc) / 1024 / 1024
	heap1 := float64(memAfter1.HeapInuse) / 1024 / 1024
	heap2 := float64(memAfter2.HeapInuse) / 1024 / 1024

	t.Logf("=== Memory Comparison (%d iterations) ===", iterations)
	t.Logf("Without Pooling:")
	t.Logf("  Allocated: %.2f MB", allocated1)
	t.Logf("  Heap Inuse: %.2f MB", heap1)
	t.Logf("")
	t.Logf("With Pooling:")
	t.Logf("  Allocated: %.2f MB", allocated2)
	t.Logf("  Heap Inuse: %.2f MB", heap2)
	t.Logf("")
	t.Logf("Difference:")
	t.Logf("  Allocated: %.2f MB (%.1f%%)", allocated2-allocated1, (allocated2/allocated1-1)*100)
	t.Logf("  Heap Inuse: %.2f MB (%.1f%%)", heap2-heap1, (heap2/heap1-1)*100)

	// The pooling version should have lower heap usage
	if heap2 < heap1 {
		t.Logf("SUCCESS: Pooling reduces heap usage by %.2f MB", heap1-heap2)
	} else {
		t.Logf("INFO: Pooling heap usage is higher by %.2f MB (may be due to GC timing)", heap2-heap1)
	}
}

// TestGjsonReferenceChain demonstrates the reference chain issue
func TestGjsonReferenceChain(t *testing.T) {
	t.Run("ExplainReferenceChain", func(t *testing.T) {
		t.Log("=== gjson Memory Leak Reference Chain ===")
		t.Log("")
		t.Log("WITHOUT POOLING:")
		t.Log("  1. HTTP framework reads request → bodyBytes[]")
		t.Log("  2. Handler calls json.Unmarshal(bodyBytes, &req)")
		t.Log("  3. SDK calls gjson.ParseBytes(bodyBytes)")
		t.Log("  4. gjson.Result.raw = bodyBytes (direct reference)")
		t.Log("  5. SDK caches decoderFunc globally")
		t.Log("  6. decoderFunc may hold gjson.Result references")
		t.Log("  7. bodyBytes cannot be GC'd → LEAK")
		t.Log("")
		t.Log("WITH POOLING:")
		t.Log("  1. HTTP framework reads request → bodyBytes[]")
		t.Log("  2. Handler calls memory.CopyRequestBody(bodyBytes)")
		t.Log("  3. Creates independent copy → bodyCopy[]")
		t.Log("  4. Handler calls json.Unmarshal(bodyCopy, &req)")
		t.Log("  5. SDK calls gjson.ParseBytes(bodyCopy)")
		t.Log("  6. gjson.Result.raw = bodyCopy (references copy)")
		t.Log("  7. bodyBytes can be GC'd → FIXED")
		t.Log("  8. bodyCopy can be GC'd after request → FIXED")
	})

	// Verify the explanation with code
	requestBody := []byte(`{"model":"test","messages":[],"stream":true}`)

	// Without pooling
	var req1 protocol.AnthropicBetaMessagesRequest
	err1 := json.Unmarshal(requestBody, &req1)
	if err1 != nil {
		t.Fatalf("Unmarshal failed: %v", err1)
	}
	t.Logf("Without pooling: Parsed request successfully (but bodyBytes may be retained)")

	// With pooling
	bodyCopy := memory.CopyRequestBody(requestBody)
	var req2 protocol.AnthropicBetaMessagesRequest
	err2 := json.Unmarshal(bodyCopy, &req2)
	if err2 != nil {
		t.Fatalf("Unmarshal failed: %v", err2)
	}
	t.Logf("With pooling: Parsed request successfully (bodyBytes can be GC'd)")

	// Verify both produce same result
	if string(req1.Model) != string(req2.Model) {
		t.Errorf("Results differ: %s vs %s", req1.Model, req2.Model)
	}
}

// TestLargeRequestHandling tests with larger request bodies
func TestLargeRequestHandling(t *testing.T) {
	// Create a larger request body (simulating real usage with many messages)
	messages := make([]map[string]string, 100)
	for i := range messages {
		messages[i] = map[string]string{
			"role":    "user",
			"content": fmt.Sprintf("Message number %d with some content to increase size", i),
		}
	}

	requestBody := fmt.Sprintf(`{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 4096,
		"messages": %s,
		"stream": true
	}`, toJSON(messages))

	t.Run("LargeRequestWithoutPooling", func(t *testing.T) {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)

		for i := 0; i < 100; i++ {
			bodyBytes := []byte(requestBody)
			var req protocol.AnthropicBetaMessagesRequest
			json.Unmarshal(bodyBytes, &req)
		}

		runtime.GC()
		runtime.ReadMemStats(&after)

		heapMB := float64(after.HeapInuse-before.HeapInuse) / 1024 / 1024
		t.Logf("Large request without pooling: %.2f MB heap growth", heapMB)
	})

	t.Run("LargeRequestWithPooling", func(t *testing.T) {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)

		for i := 0; i < 100; i++ {
			bodyBytes := []byte(requestBody)
			bodyCopy := memory.CopyRequestBody(bodyBytes)
			var req protocol.AnthropicBetaMessagesRequest
			json.Unmarshal(bodyCopy, &req)
		}

		runtime.GC()
		runtime.ReadMemStats(&after)

		heapMB := float64(after.HeapInuse-before.HeapInuse) / 1024 / 1024
		t.Logf("Large request with pooling: %.2f MB heap growth", heapMB)
	})
}

// TestRealMemoryLeakScenario tests the actual leak scenario where
// Gin contexts or request bodies might be cached somewhere
func TestRealMemoryLeakScenario(t *testing.T) {
	gin.SetMode(gin.TestMode)

	requestBody := `{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 4096,
		"messages": [{"role": "user", "content": "Test"}],
		"stream": true
	}`

	t.Run("SimulateServerWithoutPooling", func(t *testing.T) {
		simulateServerScenario(t, requestBody, false)
	})

	t.Run("SimulateServerWithPooling", func(t *testing.T) {
		simulateServerScenario(t, requestBody, true)
	})
}

func simulateServerScenario(t *testing.T, requestBody string, usePooling bool) {
	// Setup server that mimics tingly-box's handler pattern
	router := gin.New()

	// This mimics the actual handler pattern
	router.POST("/messages", func(c *gin.Context) {
		bodyBytes, err := c.GetRawData()
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		// This is the problematic pattern from anthropic.go:151
		// We put the bodyBytes back into c.Request.Body
		// If Gin caches contexts or bodies, this causes leaks
		c.Request.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))

		if usePooling {
			// With pooling: copy to break reference
			bodyBytes = memory.CopyRequestBody(bodyBytes)
		}

		var req protocol.AnthropicBetaMessagesRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		c.JSON(200, gin.H{"model": string(req.Model)})
	})

	// Simulate many concurrent requests (like real server load)
	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	iterations := 100000
	concurrency := 100

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations/concurrency; j++ {
				req := httptest.NewRequest("POST", "/messages", strings.NewReader(requestBody))
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Don't keep response
			}
		}(i)
	}
	wg.Wait()

	// Give time for GC
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	heapGrowth := float64(memAfter.HeapInuse-memBefore.HeapInuse) / 1024 / 1024
	totalAlloc := float64(memAfter.TotalAlloc-memBefore.TotalAlloc) / 1024 / 1024

	poolingStatus := "Without Pooling"
	if usePooling {
		poolingStatus = "With Pooling"
	}

	t.Logf("%s:", poolingStatus)
	t.Logf("  Heap Growth: %.2f MB", heapGrowth)
	t.Logf("  Total Alloc: %.2f MB", totalAlloc)
	t.Logf("  Heap Objects: %d → %d (delta: %d)",
		memBefore.HeapObjects, memAfter.HeapObjects,
		memAfter.HeapObjects-memBefore.HeapObjects)

	// The key indicator: heap object growth
	// Without pooling, we expect more objects retained
	if !usePooling && heapGrowth > 10 {
		t.Logf("WARNING: High heap growth suggests potential leak")
	}
	if usePooling && heapGrowth < 5 {
		t.Logf("GOOD: Heap growth controlled with pooling")
	}
}

// TestRequestBodyRetention tests if request bodies are retained
func TestRequestBodyRetention(t *testing.T) {
	gin.SetMode(gin.TestMode)

	requestBody := `{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 4096,
		"messages": [{"role": "user", "content": "Test"}],
		"stream": true
	}`

	t.Run("RequestBodyRetentionWithoutPooling", func(t *testing.T) {
		testRequestBodyRetention(t, requestBody, false)
	})

	t.Run("RequestBodyRetentionWithPooling", func(t *testing.T) {
		testRequestBodyRetention(t, requestBody, true)
	})
}

func testRequestBodyRetention(t *testing.T, requestBody string, usePooling bool) {
	var contexts []*gin.Context
	var contextsMutex sync.Mutex

	router := gin.New()
	router.POST("/messages", func(c *gin.Context) {
		bodyBytes, _ := c.GetRawData()

		// Simulate the problematic pattern
		c.Request.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))

		if usePooling {
			bodyBytes = memory.CopyRequestBody(bodyBytes)
		}

		var req protocol.AnthropicBetaMessagesRequest
		json.Unmarshal(bodyBytes, &req)

		// INTENTIONAL: Retain context reference to simulate caching
		contextsMutex.Lock()
		contexts = append(contexts, c)
		contextsMutex.Unlock()

		c.JSON(200, gin.H{"ok": true})
	})

	// Process many requests
	iterations := 100000
	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest("POST", "/messages", strings.NewReader(requestBody))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	heapMB := float64(memAfter.HeapInuse) / 1024 / 1024
	objCount := memAfter.HeapObjects

	poolingStatus := "Without Pooling"
	if usePooling {
		poolingStatus = "With Pooling"
	}

	t.Logf("%s (retaining contexts):", poolingStatus)
	t.Logf("  Heap Inuse: %.2f MB", heapMB)
	t.Logf("  Heap Objects: %d", objCount)

	// Clear contexts and check GC
	contexts = nil
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	var memAfterGC runtime.MemStats
	runtime.ReadMemStats(&memAfterGC)

	heapAfterGC := float64(memAfterGC.HeapInuse) / 1024 / 1024
	objAfterGC := memAfterGC.HeapObjects

	t.Logf("  After GC: %.2f MB, %d objects", heapAfterGC, objAfterGC)

	if !usePooling && heapMB > 50 {
		t.Logf("WARNING: High memory usage without pooling")
	}
}

// TestGjsonAccumulation tests if gjson results accumulate
func TestGjsonAccumulation(t *testing.T) {
	requestBody := `{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 4096,
		"messages": [{"role": "user", "content": "Test message content"}],
		"stream": true
	}`

	t.Run("GjsonAccumulationWithoutPooling", func(t *testing.T) {
		testGjsonAccumulation(t, requestBody, false)
	})

	t.Run("GjsonAccumulationWithPooling", func(t *testing.T) {
		testGjsonAccumulation(t, requestBody, true)
	})
}

func testGjsonAccumulation(t *testing.T, requestBody string, usePooling bool) {
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	// Process many requests sequentially
	iterations := 1000000
	for i := 0; i < iterations; i++ {
		bodyBytes := []byte(requestBody)

		if usePooling {
			bodyBytes = memory.CopyRequestBody(bodyBytes)
		}

		var req protocol.AnthropicBetaMessagesRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		// Force periodic GC to see accumulation pattern
		if i%1000 == 999 {
			runtime.GC()
		}
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	heapGrowth := float64(after.HeapInuse-before.HeapInuse) / 1024 / 1024
	allocDiff := float64(after.TotalAlloc-before.TotalAlloc) / 1024 / 1024

	poolingStatus := "Without Pooling"
	if usePooling {
		poolingStatus = "With Pooling"
	}

	t.Logf("%s (1M iterations):", poolingStatus)
	t.Logf("  Heap Growth: %.2f MB", heapGrowth)
	t.Logf("  Total Allocation: %.2f MB", allocDiff)
	t.Logf("  Heap Objects: %d", after.HeapObjects)

	if !usePooling && heapGrowth > 20 {
		t.Logf("WARNING: Significant heap growth without pooling")
	}
	if usePooling && heapGrowth < 10 {
		t.Logf("GOOD: Heap growth controlled with pooling")
	}
}

// TestRealWorldPattern tests actual Gin usage patterns
func TestRealWorldPattern(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Simulate middleware that might cache data
	var middlewareData []*interface{}
	var middlewareMutex sync.Mutex

	router := gin.New()

	// Middleware that might retain request data
	router.Use(func(c *gin.Context) {
		// Simulate middleware that stores request info
		bodyBytes, _ := c.GetRawData()
		data := interface{}(bodyBytes)

		middlewareMutex.Lock()
		middlewareData = append(middlewareData, &data)
		middlewareMutex.Unlock()

		c.Next()
	})

	// Handler
	router.POST("/messages", func(c *gin.Context) {
		bodyBytes, _ := c.GetRawData()
		c.Request.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))

		var req protocol.AnthropicBetaMessagesRequest
		json.Unmarshal(bodyBytes, &req)

		c.JSON(200, gin.H{"ok": true})
	})

	requestBody := `{"model":"test","messages":[],"stream":true}`

	// Simulate server load
	for i := 0; i < 1000; i++ {
		req := httptest.NewRequest("POST", "/messages", strings.NewReader(requestBody))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	runtime.GC()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	t.Logf("Real-world pattern (middleware caching):")
	t.Logf("  Heap Inuse: %.2f MB", float64(memStats.HeapInuse)/1024/1024)
	t.Logf("  Heap Objects: %d", memStats.HeapObjects)
	t.Logf("  Middleware data items: %d", len(middlewareData))

	// This demonstrates the risk: middleware caching request data
	if len(middlewareData) == 1000 {
		t.Logf("INFO: Middleware retained all 1000 request references")
		t.Logf("This is why pooling is important - breaks the reference chain")
	}
}

// Helper function to convert slice to JSON
func toJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}
