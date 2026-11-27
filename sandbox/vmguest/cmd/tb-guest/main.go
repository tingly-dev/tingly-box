// Command tb-guest is the init of a guest image (macvm's): the kernel starts
// it as process 1 with TB_SANDBOX_GUEST=1, and package vmguest runs the
// command the host sends, then powers off. Build it for the image:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o tb-guest ./vmguest/cmd/tb-guest
package main

import (
	"fmt"
	"os"

	_ "github.com/tingly-dev/tingly-box/sandbox/vmguest"
)

func main() {
	// Only reached when not a guest's init.
	fmt.Fprintln(os.Stderr, "tb-guest is the init of a sandbox VM's guest; it does nothing on its own")
	os.Exit(2)
}
