package seatbelt

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/tingly-dev/tingly-box/sandbox"
)

// systemRead is what every process needs to read to run at all: the system
// directories and the few devices. Reading is granted by subpath; macOS keeps
// /etc, /var and /tmp under /private.
var systemRead = []string{
	"/usr", "/bin", "/sbin", "/lib", "/opt", "/System", "/Library",
	"/private/etc", "/private/var/db", "/private/var/select",
	"/Applications/Xcode.app/Contents/Developer", "/Library/Developer/CommandLineTools",
}

// systemDevices are the devices a command may use.
var systemDevices = []string{"/dev/null", "/dev/zero", "/dev/random", "/dev/urandom", "/dev/dtracehelper", "/dev/tty"}

// loopbackAll is the SBPL address for the host's own loopback, as the sandbox
// sees it (it has none of its own: the network stack is the host's).
const loopbackAll = "localhost"

// plan is what a profile is built from.
type plan struct {
	policy sandbox.Policy
	// fresh is the sandbox's private directory (home and temporary files).
	fresh string
	// proxyPort is the host's loopback port of the allowlist proxy; 0 for none.
	proxyPort int
}

// profile writes the Seatbelt profile (SBPL) for pl. Rules are applied in
// order and the last matching rule wins, so the file rules go by depth: a
// longer path lands on top of every shorter one, as sandbox.md §2.1 has it.
func profile(pl plan) (string, error) {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	w("(version 1)")
	w("(deny default)")
	w("(allow process-exec)")
	w("(allow process-fork)")
	w("(allow signal (target self))")
	w("(allow sysctl-read)")
	w("(allow user-preference-read)")
	w("(allow ipc-posix-shm-read* ipc-posix-shm-write-data)")
	w(`(allow mach-lookup (global-name "com.apple.system.opendirectoryd.libinfo") (global-name "com.apple.system.logger") (global-name "com.apple.system.notification_center") (global-name "com.apple.trustd.agent") (global-name "com.apple.SecurityServer") (global-name "com.apple.logd") (global-name "com.apple.CoreServices.coreservicesd") (global-name "com.apple.lsd.mapdb") (global-name "com.apple.mDNSResponder"))`)
	w("(allow pseudo-tty)")
	w("(allow file-read-metadata)") // names exist, as bubblewrap shows parent directories
	for _, d := range systemRead {
		w("(allow file-read* (subpath %s))", quote(d))
	}
	for _, d := range systemDevices {
		w("(allow file-read* file-write* (literal %s))", quote(d))
	}
	w("(allow file-ioctl (literal \"/dev/dtracehelper\"))")

	// Storage: the sandbox's own directory, then the mounts by depth.
	if pl.policy.Storage.Level != sandbox.StorageConfined {
		return "", fmt.Errorf("seatbelt gives storage confined only")
	}
	if pl.fresh != "" {
		w("(allow file-read* file-write* (subpath %s))", quote(pl.fresh))
	}
	mounts, err := resolveMounts(pl.policy.Storage.Mounts)
	if err != nil {
		return "", err
	}
	for _, m := range mounts {
		sel := "subpath"
		if !m.dir {
			sel = "literal"
		}
		if m.writable {
			w("(allow file-read* file-write* (%s %s))", sel, quote(m.path))
		} else {
			w("(allow file-read* (%s %s))", sel, quote(m.path))
			w("(deny file-write* (%s %s))", sel, quote(m.path))
		}
	}

	// Network: nothing, except what the level and Expose name.
	switch n := pl.policy.Network; n.Level {
	case sandbox.NetworkOpen:
		w("(allow network*)")
	case sandbox.NetworkNone, sandbox.NetworkAllowlist:
		for _, e := range n.Expose {
			w("(allow network-outbound (remote ip \"%s:%d\"))", loopbackAll, e.Port)
		}
		if n.Level == sandbox.NetworkAllowlist {
			if pl.proxyPort == 0 {
				return "", fmt.Errorf("an allowlist without its proxy")
			}
			w("(allow network-outbound (remote ip \"%s:%d\"))", loopbackAll, pl.proxyPort)
		}
	default:
		return "", fmt.Errorf("seatbelt gives network none, allowlist and open only, not %s", n.Level)
	}
	return b.String(), nil
}

// quote is s as an SBPL string.
func quote(s string) string {
	s = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s)
	return `"` + s + `"`
}

// mount is one effective mount: a real path (symbolic links resolved: the
// profile matches what the kernel resolves), by depth.
type mount struct {
	path     string
	dir      bool
	writable bool
}

// resolveMounts is the effective mounts (§2.1 M*) sorted so that a longer
// path comes after the shorter ones it lies under. A missing required path
// is an error; a missing optional one is skipped.
func resolveMounts(ms []sandbox.Mount) ([]mount, error) {
	var out []mount
	for _, m := range ms {
		real, err := filepath.EvalSymlinks(m.Path)
		if err != nil {
			if os.IsNotExist(err) && m.Optional {
				continue
			}
			return nil, fmt.Errorf("mount %s: %w", m.Path, err)
		}
		fi, err := os.Stat(real)
		if err != nil {
			return nil, fmt.Errorf("mount %s: %w", m.Path, err)
		}
		out = append(out, mount{path: real, dir: fi.IsDir(), writable: m.Writable})
	}
	depth := func(p string) int { return strings.Count(p, "/") }
	sort.SliceStable(out, func(i, j int) bool { return depth(out[i].path) < depth(out[j].path) })
	return out, nil
}

// proxyEnv names the allowlist proxy at the host's loopback port.
func proxyEnv(port int) []string {
	url := "http://127.0.0.1:" + strconv.Itoa(port)
	return []string{
		"HTTP_PROXY=" + url, "HTTPS_PROXY=" + url, "http_proxy=" + url, "https_proxy=" + url,
		"NO_PROXY=127.0.0.1,localhost", "no_proxy=127.0.0.1,localhost",
	}
}
