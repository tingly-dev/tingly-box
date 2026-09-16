package posemodel

// FileStatus is one artifact as it stands on disk.
type FileStatus struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	Ready bool   `json:"ready"`
	// Error is set when a download or verification failed for this file.
	Error string `json:"error,omitempty"`
}

// StatusResponse says whether the browser can run the estimator right now,
// and where to fetch the files from if so.
type StatusResponse struct {
	Success bool `json:"success"`
	// Ready is true only when every artifact is present and the right size.
	Ready bool         `json:"ready"`
	Files []FileStatus `json:"files"`
	// BaseURL is the path the gateway serves the files under, relative to
	// its origin, with a trailing slash — what MediaPipe's file resolver
	// wants to be given.
	BaseURL string `json:"base_url"`
	// TotalBytes is the size of everything that would be downloaded, so the
	// UI can say how much before it starts.
	TotalBytes int64 `json:"total_bytes"`
}
