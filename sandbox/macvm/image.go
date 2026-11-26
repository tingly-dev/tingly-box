package macvm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// The guest image (.design/sandbox-macvm.md §5): a directory with a manifest,
// a Linux kernel and a read-only root file system (erofs) holding the
// userland and the guest init (sandbox/vmguest/cmd/tb-guest) as /sbin/tb-guest.
const (
	imageVersion = 1
	kernelFile   = "kernel"
	rootfsFile   = "rootfs.erofs"
	manifestFile = "manifest.json"
	// ImageEnv names the image directory; without it the user's cache
	// directory has one per version and architecture.
	ImageEnv = "TB_SANDBOX_MACVM_IMAGE"
)

// Manifest is manifest.json.
type Manifest struct {
	Version int    `json:"version"`
	Arch    string `json:"arch"`
	Kernel  Blob   `json:"kernel"`
	Rootfs  Blob   `json:"rootfs"`
}

// Blob is a file of the image.
type Blob struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Image is a guest image on disk.
type Image struct {
	Dir      string
	Manifest Manifest
}

// Kernel and Rootfs are the paths of the image's files.
func (i *Image) Kernel() string { return filepath.Join(i.Dir, kernelFile) }
func (i *Image) Rootfs() string { return filepath.Join(i.Dir, rootfsFile) }

// goarch is the host's architecture, which the image must match; a variable
// for tests.
var goarch = runtime.GOARCH

// imageDir is where the image is looked for.
func imageDir() (string, error) {
	if d := os.Getenv(ImageEnv); d != "" {
		return d, nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("no cache directory (%v): name the image directory in %s", err, ImageEnv)
	}
	return filepath.Join(cache, "tingly-box", "sandbox", "macvm", fmt.Sprintf("%d-%s", imageVersion, goarch)), nil
}

// LoadImage reads the manifest in dir and checks the files are there with
// the sizes it names (Verify checks their contents).
func LoadImage(dir string) (*Image, error) {
	b, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no guest image in %s: build one with sandbox/macvm/image/build.sh and put it there, or name its directory in %s", dir, ImageEnv)
		}
		return nil, err
	}
	img := &Image{Dir: dir}
	if err := json.Unmarshal(b, &img.Manifest); err != nil {
		return nil, fmt.Errorf("%s: %v", filepath.Join(dir, manifestFile), err)
	}
	m := img.Manifest
	if m.Version != imageVersion {
		return nil, fmt.Errorf("the guest image in %s is version %d, this build needs %d: rebuild it", dir, m.Version, imageVersion)
	}
	if m.Arch != goarch {
		return nil, fmt.Errorf("the guest image in %s is for %s, this machine is %s", dir, m.Arch, goarch)
	}
	for name, want := range map[string]Blob{kernelFile: m.Kernel, rootfsFile: m.Rootfs} {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("guest image: %v", err)
		}
		if fi.Size() != want.Size {
			return nil, fmt.Errorf("guest image: %s is %d bytes, the manifest says %d: rebuild or download it again", name, fi.Size(), want.Size)
		}
	}
	return img, nil
}

// Verify checks the contents of the image's files against the manifest.
func (i *Image) Verify() error {
	for name, want := range map[string]Blob{kernelFile: i.Manifest.Kernel, rootfsFile: i.Manifest.Rootfs} {
		f, err := os.Open(filepath.Join(i.Dir, name))
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		_ = f.Close()
		if err != nil {
			return err
		}
		if got := hex.EncodeToString(h.Sum(nil)); got != want.SHA256 {
			return fmt.Errorf("guest image: %s has sha256 %s, the manifest says %s", name, got, want.SHA256)
		}
	}
	return nil
}
