package vmodel

import "github.com/tingly-dev/tingly-box/vmodel/promptcache"

// PromptCacheModelID is the virtual model that simulates an upstream prompt
// cache: its usage reports the cache reads an ideal prefix cache would serve
// for the request, and its reply says the same in words. Point a client at it
// through the gateway to measure the cache hit rate the gateway path allows,
// independent of any real provider. See .design/vmodel-prompt-cache.md.
const PromptCacheModelID = "virtual-prompt-cache"

// PromptCacheModelDescription is the user-facing description of
// PromptCacheModelID, shared by both protocol registries.
const PromptCacheModelDescription = "Simulates a provider prompt cache: replies with, and reports in usage, how much of each request an ideal prefix cache would serve. Use it to check cache hit rate through the gateway."

// PromptCacheModel is implemented by virtual models whose responses are
// produced by the virtual server's prompt-cache simulation rather than by the
// model itself: the server needs the raw request body, which models never see.
type PromptCacheModel interface {
	PromptCache() *promptcache.Simulator
}
