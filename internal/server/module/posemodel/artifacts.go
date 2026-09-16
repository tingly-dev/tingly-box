// Package posemodel fetches and serves the files the browser needs to run
// MediaPipe's pose landmarker: the wasm runtime and the lite model.
//
// tingly-box is a gateway that runs on the user's own machine, often inside a
// network that cannot reach a CDN, so the browser is never pointed at one. The
// files are downloaded once, on first use, into the config directory, verified
// against pinned hashes, and served by this gateway from then on. Shipping
// them inside the binary was the other option and was rejected: every user
// would carry eighteen megabytes on every upgrade for a feature most never
// touch (see .design/pose-from-image.md §6).
package posemodel

// Artifact is one file the browser needs, pinned by hash: the download source
// may move or change, and a pose estimator that silently changed underneath a
// user is worse than one that refuses to run.
type Artifact struct {
	Name   string
	URL    string
	SHA256 string
	Size   int64
}

// DirName is the folder under the config directory the files live in.
const DirName = "pose-model"

// Artifacts is everything `ensure` fetches. The wasm runtime is the SIMD
// build only: every browser that can run this UI has had wasm SIMD for years,
// and carrying the fallback would double the download for nobody.
//
// Hashes were taken from @mediapipe/tasks-vision@1.0.1 as installed from npm
// and from the model as served by Google's bucket on 2026-09-16.
var Artifacts = []Artifact{
	{
		Name:   "vision_wasm_internal.js",
		URL:    "https://cdn.jsdelivr.net/npm/@mediapipe/tasks-vision@1.0.1/wasm/vision_wasm_internal.js",
		SHA256: "e170ee67dd4e16c1a6fcd8840a206687e5a59b22c20e4a902bc445b095454d73",
		Size:   323377,
	},
	{
		Name:   "vision_wasm_internal.wasm",
		URL:    "https://cdn.jsdelivr.net/npm/@mediapipe/tasks-vision@1.0.1/wasm/vision_wasm_internal.wasm",
		SHA256: "8da277a733926eacd0474b8704b36742d6ec3231c57a860c5b889dff8f1df886",
		Size:   11756954,
	},
	{
		Name:   "pose_landmarker_lite.task",
		URL:    "https://storage.googleapis.com/mediapipe-models/pose_landmarker/pose_landmarker_lite/float16/latest/pose_landmarker_lite.task",
		SHA256: "59929e1d1ee95287735ddd833b19cf4ac46d29bc7afddbbf6753c459690d574a",
		Size:   5777746,
	},
}

// ArtifactByName returns the pinned artifact, or nil for a name that is not
// ours — which is how the file route refuses to serve anything else.
func ArtifactByName(name string) *Artifact {
	for i := range Artifacts {
		if Artifacts[i].Name == name {
			return &Artifacts[i]
		}
	}
	return nil
}
