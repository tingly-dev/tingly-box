package protocoltest

import "testing"

func TestMCPRemoteNotInjected(t *testing.T) { runServerToolTest(t, "TestMCPRemoteNotInjected") }

func TestMCPRoutingExposure(t *testing.T) { runServerToolTest(t, "TestMCPRoutingExposure") }
