package tui

import (
	"io"
	"testing"
)

// scriptedIO returns a testIOFactory (see WithTestIO) that hands each
// successive prompt a fresh pipe pre-loaded with the next entry in steps —
// e.g. "y" for a Confirm, "\r" to accept a Select's default. A fresh pipe
// per call matters: see the testIOFactory doc comment in tui.go for why
// reusing one pipe across chained prompts hangs the second one.
func scriptedIO(t *testing.T, steps ...string) func() (io.Reader, io.Writer) {
	t.Helper()
	next := 0
	return func() (io.Reader, io.Writer) {
		var script string
		if next < len(steps) {
			script = steps[next]
			next++
		}
		r, w := io.Pipe()
		go func() {
			_, _ = w.Write([]byte(script))
			_ = w.Close()
		}()
		return r, io.Discard
	}
}

// TestWithTestIO_ChainsMultiplePrompts proves the run() injection point
// works across a *sequence* of prompts, not just one. Every mode loop
// (RunProviderMode, RunRuleMode, ...) calls Select/Input/Confirm one after
// another, each spinning up its own tea.Program via run(); a redesign of
// one of those flows can only be verified end-to-end if a test can script
// keystrokes across that whole sequence in one pass, not just unit-test the
// business logic underneath each individual prompt (the pattern the
// existing *_test.go files in this package use today).
func TestWithTestIO_ChainsMultiplePrompts(t *testing.T) {
	WithTestIO(t, scriptedIO(t,
		"y",  // Confirm: choose Yes
		"\r", // Select: accept first item (cursor default)
	))

	confirmResult, err := Confirm("Proceed with the risky thing?")
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if !confirmResult.IsConfirm() || !confirmResult.Value {
		t.Fatalf("Confirm result = %+v, want confirmed Yes", confirmResult)
	}

	selectResult, err := Select("Pick one", []SelectItem[string]{
		{Title: "first", Value: "first"},
		{Title: "second", Value: "second"},
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if !selectResult.IsConfirm() || selectResult.Value != "first" {
		t.Fatalf("Select result = %+v, want confirmed \"first\"", selectResult)
	}
}

// TestWithTestIO_Input covers the free-text prompt, and that WithTestIO's
// t.Cleanup actually restores production behavior once the subtest ends.
func TestWithTestIO_Input(t *testing.T) {
	t.Run("scripted", func(t *testing.T) {
		WithTestIO(t, scriptedIO(t, "hello\r"))

		result, err := Input("Name?")
		if err != nil {
			t.Fatalf("Input: %v", err)
		}
		if !result.IsConfirm() || result.Value != "hello" {
			t.Fatalf("Input result = %+v, want confirmed \"hello\"", result)
		}
	})

	if testIOFactory != nil {
		t.Fatal("testIOFactory leaked past the subtest that set it")
	}
}
