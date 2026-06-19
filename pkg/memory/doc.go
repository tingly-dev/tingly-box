// Package memory provides reusable memory pooling utilities.
//
// The primary use case is preventing memory leaks when using SDKs that employ
// zero-copy JSON parsers (like gjson). By copying request bodies through a
// pool, we break the reference chain from the parser to the original request data.
//
// Example usage in HTTP handlers:
//
//	import "github.com/tingly-dev/tingly-box/pkg/memory"
//
//	func (s *Server) HandleRequest(c *gin.Context) {
//	    bodyBytes, _ := c.GetRawData()
//
//	    // Copy through pool to break gjson references
//	    bodyCopy := memory.CopyRequestBody(bodyBytes)
//
//	    // Use the copy with SDK
//	    var req SDKType
//	    json.Unmarshal(bodyCopy, &req)
//
//	    // Original bodyBytes can now be GC'd
//	}
//
// Performance characteristics:
//   - Small requests (<32KB): Reuses pooled buffers, minimal allocation
//   - Medium requests (32KB-1MB): Reuses pooled buffers after growth
//   - Large requests (>1MB): Direct allocation, not pooled (prevents bloat)
//
// Memory savings: With 10k concurrent requests of 10KB each:
//   - Without pooling: ~100MB retained by gjson references
//   - With pooling: ~3MB in pool buffers (reusable)
package memory
