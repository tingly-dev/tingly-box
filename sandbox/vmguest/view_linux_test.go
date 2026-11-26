//go:build linux

package vmguest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The guest builds its own view (macvm's way) on a real VM: Cloud Hypervisor
// boots a read-only ext4 root holding only tb-guest and a static busybox,
// with a virtio-fs share, and the command checks what it sees. Runs where
// KVM, Cloud Hypervisor, virtiofsd (TB_SANDBOX_VMM, TB_SANDBOX_VIRTIOFSD), a
// guest kernel, mkfs.ext4 and a static busybox are.
func TestGuestBuildsItsOwnView(t *testing.T) {
	need := func(env, fallback string) string {
		p := os.Getenv(env)
		if p == "" {
			p = fallback
		}
		if _, err := os.Stat(p); err != nil {
			t.Skipf("%s: %v", env, err)
		}
		return p
	}
	vmm := need("TB_SANDBOX_VMM", "/opt/kata/bin/cloud-hypervisor")
	fsd := need("TB_SANDBOX_VIRTIOFSD", "/opt/kata/libexec/virtiofsd")
	kernel := need("TB_SANDBOX_KERNEL", "/opt/kata/share/kata-containers/vmlinux.container")
	busybox := need("TB_SANDBOX_BUSYBOX", "/bin/busybox")
	if f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0); err != nil {
		t.Skip(err)
	} else {
		_ = f.Close()
	}
	mkfs, err := exec.LookPath("mkfs.ext4")
	if err != nil {
		t.Skip(err)
	}
	if out, err := exec.Command("file", "-L", busybox).Output(); err != nil || !bytes.Contains(out, []byte("statically linked")) {
		t.Skipf("%s is not a static busybox", busybox)
	}

	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	for _, d := range []string{"sbin", "bin", "proc", "sys", "dev", "mnt", "etc", "tmp", "run"} {
		must(t, os.MkdirAll(filepath.Join(root, d), 0o755))
	}
	build := exec.Command("go", "build", "-o", filepath.Join(root, "sbin/tb-guest"), "./cmd/tb-guest")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build tb-guest: %v\n%s", err, out)
	}
	data, err := os.ReadFile(busybox)
	must(t, err)
	must(t, os.WriteFile(filepath.Join(root, "bin/busybox"), data, 0o755))
	for _, tool := range []string{"sh", "cat", "id", "ls", "echo", "test", "stat"} {
		must(t, os.Symlink("busybox", filepath.Join(root, "bin", tool)))
	}
	must(t, os.WriteFile(filepath.Join(root, "etc/passwd"), []byte("root:x:0:0:root:/root:/bin/sh\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(root, "etc/group"), []byte("root:x:0:\n"), 0o644))
	img := filepath.Join(dir, "root.img")
	if out, err := exec.Command(mkfs, "-q", "-F", "-d", root, img, "64M").CombinedOutput(); err != nil {
		t.Fatalf("mkfs: %v\n%s", err, out)
	}

	// The share: a writable directory, a read-only one and a file.
	share := filepath.Join(dir, "share")
	must(t, os.MkdirAll(filepath.Join(share, "0"), 0o755))
	must(t, os.MkdirAll(filepath.Join(share, "1"), 0o755))
	must(t, os.MkdirAll(filepath.Join(share, "f"), 0o755))
	must(t, os.WriteFile(filepath.Join(share, "1/ro.txt"), []byte("ro\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(share, "f/conf"), []byte("conf\n"), 0o644))

	// Sockets where their paths stay short enough.
	sockDir := dir
	if rt := os.Getenv("XDG_RUNTIME_DIR"); rt != "" {
		d, err := os.MkdirTemp(rt, "tb-vmguest-")
		must(t, err)
		defer os.RemoveAll(d)
		sockDir = d
	}
	sock := filepath.Join(sockDir, "fs.sock")
	fs := exec.Command(fsd, "--socket-path="+sock, "--shared-dir="+share, "--sandbox=none", "--cache=auto", "--log-level=error")
	fs.Stderr = os.Stderr
	must(t, fs.Start())
	defer func() { _ = fs.Process.Kill(); _ = fs.Wait() }()
	for i := 0; i < 250; i++ {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	vsock := filepath.Join(sockDir, "vsock.sock")
	conns := map[int]chan net.Conn{}
	for _, port := range []int{PortControl, PortStdin, PortStdout, PortStderr} {
		ln, err := net.Listen("unix", vsock+"_"+strconv.Itoa(port))
		must(t, err)
		defer ln.Close()
		ch := make(chan net.Conn, 1)
		conns[port] = ch
		go func() {
			if c, err := ln.Accept(); err == nil {
				ch <- c
			}
		}()
	}
	console := filepath.Join(dir, "console.log")
	vm := exec.Command(vmm, "--kernel", kernel, "--cpus", "boot=1", "--memory", "size=512M,shared=on",
		"--disk", "path="+img+",readonly=on", "--fs", "tag=tb,socket="+sock,
		"--vsock", "cid=3,socket="+vsock, "--serial", "file="+console, "--console", "off",
		"--cmdline", "console=ttyS0 quiet panic=-1 root=/dev/vda ro rootfstype=ext4 init=/sbin/tb-guest "+Env+"=1")
	vm.Stderr = os.Stderr
	must(t, vm.Start())
	defer func() { _ = vm.Process.Kill(); _ = vm.Wait() }()
	accept := func(port int) net.Conn {
		select {
		case c := <-conns[port]:
			return c
		case <-time.After(30 * time.Second):
			b, _ := os.ReadFile(console)
			t.Fatalf("the guest did not connect to port %d; console:\n%s", port, b)
			return nil
		}
	}

	home := "/home/tester"
	script := strings.Join([]string{
		"id -un",
		"echo home > $HOME/x && cat $HOME/x",
		"stat -c %U $HOME",
		"cat /work/ro/ro.txt",
		"echo no 2>/dev/null > /work/ro/ro.txt || echo ro refused",
		"echo rw > /work/rw/new.txt && echo rw written",
		"cat /work/rw/conf",
		"ls /.tb-share 2>/dev/null || echo share gone",
		"cat /etc/resolv.conf",
	}, "; ")
	ctl := accept(PortControl)
	must(t, json.NewEncoder(ctl).Encode(Spec{
		// A host path the guest cannot run: the guest's own sh runs.
		Args: []string{"/Users/tester/bin/sh", "-c", script},
		Env:  []string{"PATH=/bin", "HOME=" + home}, Dir: "/",
		UID: os.Getuid(), GID: os.Getgid(),
		Root: true, Fresh: []string{home, "/tmp", "/run"}, Tag: "tb",
		Mounts: []Mount{
			{Source: "f/conf", Path: "/work/rw/conf", ReadOnly: true},
			{Source: "0", Path: "/work/rw"},
			{Source: "1", Path: "/work/ro", ReadOnly: true},
		},
		ResolvConf: "nameserver 169.254.1.53\n", User: "tester", Foreign: true,
	}))
	stdin, stdout, stderr := accept(PortStdin), accept(PortStdout), accept(PortStderr)
	_ = stdin.Close()
	var errOut bytes.Buffer
	go func() { _, _ = io.Copy(&errOut, stderr) }()
	out, _ := io.ReadAll(stdout)
	line, err := bufio.NewReader(ctl).ReadBytes('\n')
	if err != nil {
		b, _ := os.ReadFile(console)
		t.Fatalf("no result: %v; console:\n%s", err, b)
	}
	var r Result
	must(t, json.Unmarshal(line, &r))
	if r != (Result{}) {
		b, _ := os.ReadFile(console)
		t.Fatalf("result %+v; stdout:\n%s\nstderr:\n%s\nconsole:\n%s", r, out, errOut.String(), b)
	}
	want := "tester\nhome\ntester\nro\nro refused\nrw written\nconf\nshare gone\nnameserver 169.254.1.53\n"
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s\nstderr:\n%s", out, want, errOut.String())
	}
	if b, err := os.ReadFile(filepath.Join(share, "0/new.txt")); err != nil || string(b) != "rw\n" {
		t.Fatalf("the write did not reach the host: %q, %v", b, err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
