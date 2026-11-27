//go:build linux

package vmguest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// shareRoot is where the guest mounts the share while it builds the view; it
// is gone before the command starts.
const shareRoot = "/.tb-share"

// buildView makes the guest's file system what s describes: a writable
// overlay on the read-only root, the host user in passwd, resolv.conf, then
// the fresh directories and the share's mounts. Nothing to do for a guest
// whose view the host built (microvm).
func buildView(s *Spec) error {
	if s.Root {
		if err := overlayRoot(); err != nil {
			return fmt.Errorf("root overlay: %w", err)
		}
	}
	if s.User != "" {
		home := envOf(s.Env, "HOME")
		if err := editFile("/etc/passwd", func(c string) string { return withUser(c, s.UID, passwdLine(s.User, s.UID, s.GID, home)) }); err != nil {
			return err
		}
		if err := editFile("/etc/group", func(c string) string { return withUser(c, s.GID, groupLine(s.User, s.GID)) }); err != nil {
			return err
		}
	}
	if s.ResolvConf != "" {
		_ = os.Remove("/etc/resolv.conf") // often a link into /run
		if err := os.WriteFile("/etc/resolv.conf", []byte(s.ResolvConf), 0o644); err != nil {
			return err
		}
	}
	if len(s.Mounts) > 0 {
		if s.Tag == "" {
			return errors.New("mounts without a share")
		}
		if err := os.MkdirAll(shareRoot, 0o700); err != nil {
			return err
		}
		if err := syscall.Mount(s.Tag, shareRoot, "virtiofs", syscall.MS_NOSUID|syscall.MS_NODEV, ""); err != nil {
			return fmt.Errorf("mount the share %s: %w", s.Tag, err)
		}
	}
	for _, st := range viewSteps(s) {
		if err := mountStep(st, s.UID, s.GID); err != nil {
			return err
		}
	}
	if len(s.Mounts) > 0 {
		if err := syscall.Unmount(shareRoot, syscall.MNT_DETACH); err != nil {
			return fmt.Errorf("unmount the share: %w", err)
		}
		_ = os.Remove(shareRoot)
	}
	return nil
}

// mountStep mounts one step at its path, making the mount point (owned by
// the user, as bubblewrap's are) when it is missing.
func mountStep(st step, uid, gid int) error {
	if st.fresh {
		if err := mkdirAll(st.path, uid, gid); err != nil {
			return err
		}
		opts := fmt.Sprintf("mode=0755,uid=%d,gid=%d", uid, gid)
		if st.path == "/tmp" || st.path == "/run" {
			opts = "mode=1777"
		}
		if err := syscall.Mount("tmpfs", st.path, "tmpfs", syscall.MS_NOSUID|syscall.MS_NODEV, opts); err != nil {
			return fmt.Errorf("tmpfs %s: %w", st.path, err)
		}
		return nil
	}
	src := filepath.Join(shareRoot, filepath.Clean("/"+st.m.Source))
	fi, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("%s: %w", st.path, err)
	}
	if fi.IsDir() {
		err = mkdirAll(st.path, uid, gid)
	} else if err = mkdirAll(filepath.Dir(st.path), uid, gid); err == nil {
		if _, serr := os.Lstat(st.path); serr != nil {
			var f *os.File
			if f, err = os.OpenFile(st.path, os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
				_ = f.Close()
				_ = os.Lchown(st.path, uid, gid)
			}
		}
	}
	if err != nil {
		return fmt.Errorf("mount point %s: %w", st.path, err)
	}
	flags := uintptr(syscall.MS_BIND | syscall.MS_REC)
	if err := syscall.Mount(src, st.path, "", flags, ""); err != nil {
		return fmt.Errorf("bind %s: %w", st.path, err)
	}
	// The host enforces read-only on the share; the guest's kernel too.
	flags = syscall.MS_BIND | syscall.MS_REMOUNT | syscall.MS_NOSUID | syscall.MS_NODEV
	if st.m.ReadOnly {
		flags |= syscall.MS_RDONLY
	}
	if err := syscall.Mount("", st.path, "", flags, ""); err != nil {
		return fmt.Errorf("remount %s: %w", st.path, err)
	}
	return nil
}

// mkdirAll makes dir and its missing parents, owned by the user.
func mkdirAll(dir string, uid, gid int) error {
	if fi, err := os.Stat(dir); err == nil {
		if !fi.IsDir() {
			return fmt.Errorf("%s is not a directory", dir)
		}
		return nil
	}
	if err := mkdirAll(filepath.Dir(dir), uid, gid); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o755); err != nil && !os.IsExist(err) {
		return err
	}
	return os.Lchown(dir, uid, gid)
}

// overlayRoot puts a writable overlay, in the guest's memory, on top of the
// read-only root and makes it the root; /proc, /sys and /dev move along.
func overlayRoot() error {
	const stage = "/mnt"
	if err := syscall.Mount("tmpfs", stage, "tmpfs", 0, "mode=0700"); err != nil {
		return fmt.Errorf("tmpfs: %w", err)
	}
	for _, d := range []string{"upper", "work", "root"} {
		if err := os.Mkdir(stage+"/"+d, 0o755); err != nil {
			return err
		}
	}
	root := stage + "/root"
	if err := syscall.Mount("overlay", root, "overlay", 0, "lowerdir=/,upperdir="+stage+"/upper,workdir="+stage+"/work"); err != nil {
		return fmt.Errorf("overlay: %w", err)
	}
	for _, d := range []string{"/proc", "/sys", "/dev"} {
		if err := syscall.Mount(d, root+d, "", syscall.MS_MOVE, ""); err != nil {
			return fmt.Errorf("move %s: %w", d, err)
		}
	}
	if err := os.Chdir(root); err != nil {
		return err
	}
	if err := syscall.PivotRoot(".", "mnt"); err != nil {
		return fmt.Errorf("pivot_root: %w", err)
	}
	if err := syscall.Chroot("."); err != nil {
		return err
	}
	if err := os.Chdir("/"); err != nil {
		return err
	}
	return syscall.Unmount("/mnt", syscall.MNT_DETACH)
}

func editFile(path string, edit func(string) string) error {
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte(edit(string(b))), 0o644)
}

func envOf(env []string, name string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(env[i], name+"="); ok {
			return v
		}
	}
	return ""
}

// readHead is the start of the file at path, empty when it cannot be read.
func readHead(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	b := make([]byte, 4)
	n, _ := f.Read(b)
	return b[:n]
}
