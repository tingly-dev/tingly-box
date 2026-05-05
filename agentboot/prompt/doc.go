// Package prompt holds test doubles and helpers for the
// [agentboot.Prompter] contract.
//
// The Prompter interface itself lives in [agentboot] (see
// agentboot/run.go) so that the runner-side [agentboot.RunWithPrompter]
// helper can reference it without an import cycle. This package
// supplements that with scriptable test implementations.
//
// See [agentboot.Prompter] for the full timeout / AlwaysAllow contract.
package prompt
