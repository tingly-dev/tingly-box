//go:build darwin && cgo

package macvm

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/Code-Hex/vz/v3"
)

// The hypervisor is Apple's Virtualization.framework, through Code-Hex/vz.
// The process needs the com.apple.security.virtualization entitlement
// (vz.entitlements; .design/sandbox-macvm.md §6).
func init() { backend = vzBackend{} }

type vzBackend struct{}

// Check reads the entitlements the running binary was signed with: without
// the virtualization one the framework refuses every VM with an error that
// says nothing of the cause.
func (vzBackend) Check() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	out, err := exec.Command("/usr/bin/codesign", "-d", "--entitlements", ":-", exe).Output()
	if err != nil || !strings.Contains(string(out), "com.apple.security.virtualization") {
		return fmt.Errorf("this program is not signed with the com.apple.security.virtualization entitlement, which Virtualization.framework needs: sign it with `codesign -f -s - --entitlements sandbox/macvm/vz.entitlements %s`", exe)
	}
	return nil
}

func (vzBackend) Limits() (cpus, memoryMB int) {
	return int(vz.VirtualMachineConfigurationMaximumAllowedCPUCount()), int(vz.VirtualMachineConfigurationMaximumAllowedMemorySize() >> 20)
}

func (vzBackend) New(c vmConfig) (vm, error) {
	boot, err := vz.NewLinuxBootLoader(c.Kernel, vz.WithCommandLine(c.Cmdline))
	if err != nil {
		return nil, fmt.Errorf("boot loader: %w", err)
	}
	cfg, err := vz.NewVirtualMachineConfiguration(boot, uint(c.CPUs), uint64(c.MemoryMB)<<20)
	if err != nil {
		return nil, fmt.Errorf("configuration: %w", err)
	}

	disk, err := vz.NewDiskImageStorageDeviceAttachment(c.Rootfs, true)
	if err != nil {
		return nil, fmt.Errorf("root disk: %w", err)
	}
	blk, err := vz.NewVirtioBlockDeviceConfiguration(disk)
	if err != nil {
		return nil, fmt.Errorf("root disk: %w", err)
	}
	cfg.SetStorageDevicesVirtualMachineConfiguration([]vz.StorageDeviceConfiguration{blk})

	console, err := vz.NewFileSerialPortAttachment(c.Console, false)
	if err != nil {
		return nil, fmt.Errorf("console: %w", err)
	}
	port, err := vz.NewVirtioConsoleDeviceSerialPortConfiguration(console)
	if err != nil {
		return nil, fmt.Errorf("console: %w", err)
	}
	cfg.SetSerialPortsVirtualMachineConfiguration([]*vz.VirtioConsoleDeviceSerialPortConfiguration{port})

	sock, err := vz.NewVirtioSocketDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("vsock: %w", err)
	}
	cfg.SetSocketDevicesVirtualMachineConfiguration([]vz.SocketDeviceConfiguration{sock})

	rng, err := vz.NewVirtioEntropyDeviceConfiguration()
	if err != nil {
		return nil, fmt.Errorf("entropy: %w", err)
	}
	cfg.SetEntropyDevicesVirtualMachineConfiguration([]*vz.VirtioEntropyDeviceConfiguration{rng})

	if len(c.Shares) > 0 {
		dirs := map[string]*vz.SharedDirectory{}
		for _, s := range c.Shares {
			d, err := vz.NewSharedDirectory(s.Path, s.ReadOnly)
			if err != nil {
				return nil, fmt.Errorf("share %s: %w", s.Path, err)
			}
			dirs[s.Name] = d
		}
		multi, err := vz.NewMultipleDirectoryShare(dirs)
		if err != nil {
			return nil, fmt.Errorf("shares: %w", err)
		}
		fs, err := vz.NewVirtioFileSystemDeviceConfiguration(c.ShareTag)
		if err != nil {
			return nil, fmt.Errorf("shares: %w", err)
		}
		fs.SetDirectoryShare(multi)
		cfg.SetDirectorySharingDevicesVirtualMachineConfiguration([]vz.DirectorySharingDeviceConfiguration{fs})
	}

	if c.Card != nil {
		att, err := vz.NewFileHandleNetworkDeviceAttachment(c.Card)
		if err != nil {
			return nil, fmt.Errorf("network card: %w", err)
		}
		card, err := vz.NewVirtioNetworkDeviceConfiguration(att)
		if err != nil {
			return nil, fmt.Errorf("network card: %w", err)
		}
		hw, err := net.ParseMAC(c.MAC)
		if err != nil {
			return nil, err
		}
		mac, err := vz.NewMACAddress(hw)
		if err != nil {
			return nil, err
		}
		card.SetMACAddress(mac)
		cfg.SetNetworkDevicesVirtualMachineConfiguration([]*vz.VirtioNetworkDeviceConfiguration{card})
	}

	if ok, err := cfg.Validate(); err != nil || !ok {
		return nil, fmt.Errorf("the VM configuration is not valid: %v", err)
	}
	machine, err := vz.NewVirtualMachine(cfg)
	if err != nil {
		return nil, err
	}
	v := &vzMachine{vm: machine, done: make(chan struct{})}
	go v.watch()
	return v, nil
}

type vzMachine struct {
	vm   *vz.VirtualMachine
	done chan struct{}
	once sync.Once
}

// watch closes done when the machine has stopped or failed.
func (v *vzMachine) watch() {
	for state := range v.vm.StateChangedNotify() {
		if state == vz.VirtualMachineStateStopped || state == vz.VirtualMachineStateError {
			v.once.Do(func() { close(v.done) })
			return
		}
	}
}

func (v *vzMachine) Listen(port uint32) (net.Listener, error) {
	devs := v.vm.SocketDevices()
	if len(devs) == 0 {
		return nil, errors.New("the VM has no vsock device")
	}
	return devs[0].Listen(port)
}

func (v *vzMachine) Start() error { return v.vm.Start() }

func (v *vzMachine) Stop() error {
	if !v.vm.CanStop() {
		return nil
	}
	return v.vm.Stop()
}

func (v *vzMachine) Done() <-chan struct{} { return v.done }
