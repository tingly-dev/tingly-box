package project

// Import only runtimes supported by this platform. In particular, Windows
// builds must not compile Unix process and VM networking implementations.
import (
	_ "github.com/tingly-dev/tingly-box/sandbox/bwrap"
	_ "github.com/tingly-dev/tingly-box/sandbox/microvm"
)
