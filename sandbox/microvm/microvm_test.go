package microvm

import (
	"testing"

	"github.com/tingly-dev/tingly-box/sandbox/conformance"
)

// Runs where KVM, Cloud Hypervisor, virtiofsd and a guest kernel are; name
// them with TB_SANDBOX_VMM, TB_SANDBOX_VIRTIOFSD and TB_SANDBOX_KERNEL when
// they are not on PATH or where Kata Containers puts them.
func TestConformance(t *testing.T) { conformance.Run(t, Runtime{}) }
