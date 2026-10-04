package protocoltest

import "testing"

func TestMCPRemoteOwnedToolLoop(t *testing.T) { runServerToolTest(t, "TestMCPRemoteOwnedToolLoop") }
func TestMCPRemoteToolError(t *testing.T)     { runServerToolTest(t, "TestMCPRemoteToolError") }

func TestMCPRoutingExposure(t *testing.T) { runServerToolTest(t, "TestMCPRoutingExposure") }
