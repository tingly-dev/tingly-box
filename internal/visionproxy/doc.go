// Package visionproxy is the vision proxy plugin: when a downstream model is
// text-only, it describes image content via a vision-capable upstream and
// replaces the image blocks with the description so the request still works.
//
// Service (service.go) resolves the effective {provider, model} for a
// request — rule level wins over scenario level — and hands it to
// VisionProxyProcessor (vision_proxy.go), which rewrites the typed request
// in place. See .design/vision-proxy.md for the full design and
// README.md for the rewrite pipeline.
package visionproxy
