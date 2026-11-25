//go:build linux

package microvm

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/tingly-dev/tingly-box/sandbox/netproxy"
)

// As the guest's init, this binary runs the command and powers off; it runs
// before anything else of the binary would.
func init() {
	if os.Getpid() != 1 || os.Getenv(guestEnv) != "1" {
		return
	}
	if err := guest(); err != nil {
		fmt.Fprintf(os.Stderr, "tb guest: %v\n", err)
	}
	syscall.Sync()
	_ = syscall.Reboot(syscall.LINUX_REBOOT_CMD_POWER_OFF)
	for {
		time.Sleep(time.Hour)
	}
}

// guest sets up the guest, takes the command from the host, runs it as the
// host user with stdio on vsock, and reports how it ended.
func guest() error {
	for _, m := range []struct{ src, dst, fs, data string }{
		{"proc", "/proc", "proc", ""},
		{"sysfs", "/sys", "sysfs", ""},
		{"devtmpfs", "/dev", "devtmpfs", ""},
	} {
		if err := syscall.Mount(m.src, m.dst, m.fs, syscall.MS_NOSUID|syscall.MS_NODEV, m.data); err != nil && m.fs != "devtmpfs" {
			return fmt.Errorf("mount %s: %w", m.dst, err)
		}
	}
	_ = os.MkdirAll("/dev/pts", 0o755)
	_ = syscall.Mount("devpts", "/dev/pts", "devpts", syscall.MS_NOSUID|syscall.MS_NOEXEC, "newinstance,ptmxmode=0666")
	_ = os.MkdirAll("/dev/shm", 0o1777)
	_ = syscall.Mount("tmpfs", "/dev/shm", "tmpfs", syscall.MS_NOSUID|syscall.MS_NODEV, "mode=1777")
	// As in bwrap: no namespaces inside.
	_ = os.WriteFile("/proc/sys/user/max_user_namespaces", []byte("0"), 0)
	if err := loopbackUp(); err != nil {
		return fmt.Errorf("loopback: %w", err)
	}

	ctl, err := dialHost(portControl)
	if err != nil {
		return fmt.Errorf("control: %w", err)
	}
	defer ctl.Close()
	report := func(r result) { _ = json.NewEncoder(ctl).Encode(r) }
	line, err := bufio.NewReader(ctl).ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("read the command: %w", err)
	}
	var s spec
	if err := json.Unmarshal(line, &s); err != nil || len(s.Args) == 0 {
		report(result{Error: "bad command from the host"})
		return errors.New("bad command")
	}
	var stdio [3]*os.File
	for i, port := range []int{portStdin, portStdout, portStderr} {
		if stdio[i], err = dialHost(uint32(port)); err != nil {
			report(result{Error: "stdio: " + err.Error()})
			return err
		}
	}
	if s.Net != nil {
		if err := setUpNet(s.Net); err != nil {
			report(result{Error: "network: " + err.Error()})
			return err
		}
	}
	env := s.Env
	if pl := (netproxy.Plan{Proxy: s.Proxy, Expose: s.Expose}); !pl.Empty() {
		ports := vsockPorts(pl)
		dial := func(name string) (io.ReadWriteCloser, error) { return dialHost(uint32(ports[name])) }
		if env, err = netproxy.ServeBridge(pl, dial, env); err != nil {
			report(result{Error: "network bridge: " + err.Error()})
			return err
		}
	}
	// Resolve the command with its own PATH.
	for _, kv := range env {
		if len(kv) > 5 && kv[:5] == "PATH=" {
			_ = os.Setenv("PATH", kv[5:])
		}
	}
	// Inherited by the command and all it starts: setuid gives nothing.
	if _, _, e := syscall.RawSyscall6(syscall.SYS_PRCTL, 38 /* PR_SET_NO_NEW_PRIVS */, 1, 0, 0, 0, 0); e != 0 {
		report(result{Error: "no_new_privs: " + e.Error()})
		return e
	}
	cmd := exec.Command(s.Args[0], s.Args[1:]...)
	cmd.Env, cmd.Dir = env, s.Dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdio[0], stdio[1], stdio[2]
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true,
		Credential: &syscall.Credential{Uid: uint32(s.UID), Gid: uint32(s.GID), Groups: []uint32{}}}
	oomBefore := oomKills()
	if err := cmd.Start(); err != nil {
		report(result{Error: err.Error()})
		return err
	}
	for _, f := range stdio {
		_ = f.Close()
	}
	err = cmd.Wait()
	// The guest's memory is the sandbox's: any OOM kill in it ends the
	// sandbox as oom.
	oom := oomKills() > oomBefore
	var exit *exec.ExitError
	switch {
	case err == nil && !oom:
		report(result{})
	case errors.As(err, &exit):
		if ws, ok := exit.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			report(result{Signal: int(ws.Signal()), OOM: oom})
		} else {
			report(result{Exit: exit.ExitCode(), OOM: oom})
		}
	case err == nil:
		report(result{OOM: true})
	default:
		report(result{Error: err.Error()})
	}
	return nil
}

// oomKills is how many processes the guest's OOM killer has killed.
func oomKills() int {
	b, _ := os.ReadFile("/proc/vmstat")
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "oom_kill "); ok {
			n, _ := strconv.Atoi(v)
			return n
		}
	}
	return 0
}

// dialHost connects to the host (CID 2) on a vsock port.
func dialHost(port uint32) (*os.File, error) {
	const afVsock = 40
	// Non-blocking, then waited for: a blocking connect is interrupted by the
	// Go runtime's signals (EINTR), and cannot be retried.
	fd, err := syscall.Socket(afVsock, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC|syscall.SOCK_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	sa := struct {
		family    uint16
		reserved1 uint16
		port      uint32
		cid       uint32
		flags     uint8
		zero      [3]uint8
	}{family: afVsock, port: port, cid: 2}
	fail := func(e error) (*os.File, error) {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("vsock port %d: %w", port, e)
	}
	if _, _, e := syscall.Syscall(syscall.SYS_CONNECT, uintptr(fd), uintptr(unsafe.Pointer(&sa)), unsafe.Sizeof(sa)); e != 0 && e != syscall.EINPROGRESS && e != syscall.EINTR {
		return fail(e)
	}
	for {
		pfd := [1]struct {
			fd             int32
			events, revent int16
		}{{fd: int32(fd), events: 0x4 /* POLLOUT */}}
		_, _, e := syscall.Syscall6(syscall.SYS_PPOLL, uintptr(unsafe.Pointer(&pfd[0])), 1, 0, 0, 0, 0)
		if e == syscall.EINTR {
			continue
		}
		if e != 0 {
			return fail(e)
		}
		break
	}
	if soErr, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_ERROR); err != nil || soErr != 0 {
		if err == nil {
			err = syscall.Errno(soErr)
		}
		return fail(err)
	}
	if err := syscall.SetNonblock(fd, false); err != nil {
		return fail(err)
	}
	return os.NewFile(uintptr(fd), fmt.Sprintf("vsock:%d", port)), nil
}

// loopbackUp brings lo up: the network bridge listens on it.
func loopbackUp() error {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	var ifr [40]byte
	copy(ifr[:], "lo")
	*(*uint16)(unsafe.Pointer(&ifr[16])) = syscall.IFF_UP | syscall.IFF_RUNNING
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.SIOCSIFFLAGS, uintptr(unsafe.Pointer(&ifr[0]))); e != 0 {
		return e
	}
	return nil
}

// setUpNet brings up the card and routes through it, with the host's ip (the
// guest sees /usr). The filter is outside the guest, in the namespace the
// card's tap is in.
func setUpNet(n *guestNet) error {
	ip := ""
	for _, p := range []string{"/usr/sbin/ip", "/usr/bin/ip", "/sbin/ip", "/bin/ip"} {
		if _, err := os.Stat(p); err == nil {
			ip = p
			break
		}
	}
	if ip == "" {
		return errors.New("ip is not installed on the host")
	}
	for _, args := range [][]string{
		{"link", "set", "eth0", "up"},
		{"addr", "add", n.Addr, "dev", "eth0"},
		{"route", "add", "default", "via", n.Gateway},
	} {
		if out, err := exec.Command(ip, args...).CombinedOutput(); err != nil {
			return fmt.Errorf("ip %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	return nil
}
