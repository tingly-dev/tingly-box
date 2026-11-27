//go:build darwin

package macvm

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/tingly-dev/tingly-box/sandbox"
	"github.com/tingly-dev/tingly-box/sandbox/vmguest"
)

// share is a host directory the guest gets through virtio-fs: by the host,
// read-only or not (the guest's kernel cannot change that).
type share struct {
	Name     string
	Path     string
	ReadOnly bool
}

// planMounts turns the policy's mounts into shares and the guest's mounts of
// them (.design/sandbox-macvm.md §4.1). A directory is shared as it is. A
// file is put (hard-linked, so both sides see the same inode; a read-only
// one may be copied) in a directory of its own under stage, which is shared
// instead: virtio-fs shares directories, and the file's own would give the
// guest its neighbours too. A missing optional path is left out, a missing
// required one is an error.
func planMounts(ms []sandbox.Mount, stage string) ([]share, []vmguest.Mount, error) {
	var shares []share
	var mounts []vmguest.Mount
	for i, m := range ms {
		real, err := filepath.EvalSymlinks(m.Path)
		if err != nil {
			if os.IsNotExist(err) && m.Optional {
				continue
			}
			return nil, nil, fmt.Errorf("mount %s: %w", m.Path, err)
		}
		fi, err := os.Stat(real)
		if err != nil {
			return nil, nil, fmt.Errorf("mount %s: %w", m.Path, err)
		}
		name := "m" + strconv.Itoa(i)
		gm := vmguest.Mount{Source: name, Path: filepath.Clean(m.Path), ReadOnly: !m.Writable}
		if fi.IsDir() {
			shares = append(shares, share{Name: name, Path: real, ReadOnly: !m.Writable})
		} else {
			dir := filepath.Join(stage, name)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return nil, nil, err
			}
			dst := filepath.Join(dir, filepath.Base(real))
			if err := os.Link(real, dst); err != nil {
				if m.Writable {
					return nil, nil, fmt.Errorf("mount %s: a writable file must share its inode with the host and cannot be linked into %s (%v): mount its directory", m.Path, stage, err)
				}
				if err := copyFile(real, dst); err != nil {
					return nil, nil, fmt.Errorf("mount %s: %w", m.Path, err)
				}
			}
			gm.Source = name + "/" + filepath.Base(real)
			shares = append(shares, share{Name: name, Path: dir, ReadOnly: !m.Writable})
		}
		mounts = append(mounts, gm)
	}
	return shares, mounts, nil
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// The VM's size: the host's CPUs and memory when unlimited (as microvm does),
// else just enough to give the limits, which the guest's cgroup enforces.
// Variables for tests.
var (
	guestCPUs     = runtime.NumCPU()
	guestMemoryMB = hostMemoryMB
)

// guestOverheadMB is what a limited guest has beyond its memory limit: its
// kernel, init, and the overlay and fresh directories in memory.
const guestOverheadMB = 384

func guestSize(r sandbox.Resources, maxCPUs, maxMemoryMB int) (cpus, memoryMB int) {
	cpus, memoryMB = guestCPUs, guestMemoryMB()
	if r.Level == sandbox.ResourcesLimited {
		if r.CPU > 0 {
			cpus = min(cpus, (r.CPU+999)/1000)
		}
		if r.MemoryBytes > 0 {
			memoryMB = int((r.MemoryBytes+(1<<20)-1)>>20) + guestOverheadMB
		}
	}
	if maxCPUs > 0 {
		cpus = min(cpus, maxCPUs)
	}
	if maxMemoryMB > 0 {
		memoryMB = min(memoryMB, maxMemoryMB)
	}
	return max(cpus, 1), memoryMB
}
