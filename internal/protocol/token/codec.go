package token

import (
	"sync"

	"github.com/tiktoken-go/tokenizer"
)

// codecCache memoizes tokenizer codecs per encoding. tokenizer.Get compiles a
// large regexp2 split pattern on every call (only the vocabulary is memoized
// upstream), which is expensive enough to dominate per-request setup cost, so
// each codec is built once and shared.
//
// Cached codecs must only be used for Count/Encode, which are safe for
// concurrent use; Decode lazily builds the reverse vocabulary without
// synchronization and must not be called on a shared codec.
var codecCache sync.Map // tokenizer.Encoding -> codecCacheEntry

type codecCacheEntry struct {
	codec tokenizer.Codec
	err   error
}

// getCodec returns the shared codec for the given encoding, building it on
// first use. The error (if any) is cached alongside so repeated failures do
// not retry compilation.
func getCodec(encoding tokenizer.Encoding) (tokenizer.Codec, error) {
	if v, ok := codecCache.Load(encoding); ok {
		e := v.(codecCacheEntry)
		return e.codec, e.err
	}
	codec, err := tokenizer.Get(encoding)
	v, _ := codecCache.LoadOrStore(encoding, codecCacheEntry{codec: codec, err: err})
	e := v.(codecCacheEntry)
	return e.codec, e.err
}
