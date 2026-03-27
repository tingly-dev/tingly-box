package command

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// QRDisplay represents the QR code display
type QRDisplay struct {
	URL       string
	ExpiresIn int // seconds
}

// DisplayQR displays a QR code in the terminal
// It tries multiple methods and falls back to showing the URL
func DisplayQR(qrURL string) error {
	display := &QRDisplay{
		URL:       qrURL,
		ExpiresIn: 300, // 5 minutes
	}

	// Try different display methods in order of preference
	if err := display.tryIterm2Image(); err == nil {
		return nil
	}

	if err := display.tryAsciiQR(); err == nil {
		return nil
	}

	// Fallback: just show the URL
	return display.showURL()
}

// tryIterm2Image attempts to display QR as inline image in iTerm2
func (d *QRDisplay) tryIterm2Image() error {
	// Check if we're in iTerm2
	if !isIterm2() {
		return fmt.Errorf("not in iTerm2")
	}

	// For now, we only have a URL, not actual image data
	// This would need to download the image first
	return fmt.Errorf("image data not available")
}

// tryAsciiQR attempts to use external qrcode tool for ASCII art
func (d *QRDisplay) tryAsciiQR() error {
	// Check if qrcode command is available
	if _, err := exec.LookPath("qrcode"); err != nil {
		return fmt.Errorf("qrcode command not found")
	}

	// Generate ASCII QR code using qrcode tool
	cmd := exec.Command("qrcode", "-t", "ANSI", d.URL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to generate QR: %w", err)
	}

	fmt.Println()
	return nil
}

// showURL displays the URL and instructions
func (d *QRDisplay) showURL() error {
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                     Scan the QR Code                           ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Open this URL in your browser to see the QR code:")
	fmt.Println()
	fmt.Printf("  %s\n", d.URL)
	fmt.Println()
	fmt.Println("Instructions:")
	fmt.Println("  1. Open the URL above in your browser")
	fmt.Println("  2. Scan the QR code with Weixin")
	fmt.Println("  3. Confirm the login in Weixin")
	fmt.Println()
	fmt.Printf("QR code expires in %d seconds\n", d.ExpiresIn)
	fmt.Println()

	// Try to open the URL in the browser automatically
	d.openBrowser()

	return nil
}

// isIterm2 checks if we're running in iTerm2
func isIterm2() bool {
	// Check ITERM_SESSION_ID environment variable
	return os.Getenv("ITERM_SESSION_ID") != ""
}

// openBrowser opens the QR URL in the default browser
func (d *QRDisplay) openBrowser() {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{d.URL}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", d.URL}
	case "linux":
		// Try xdg-open first
		if _, err := exec.LookPath("xdg-open"); err == nil {
			cmd = "xdg-open"
			args = []string{d.URL}
		} else if _, err := exec.LookPath("wslview"); err == nil {
			// Fallback for WSL
			cmd = "wslview"
			args = []string{d.URL}
		} else {
			// Can't auto-open
			return
		}
	default:
		return
	}

	// Execute command in background
	exec.Command(cmd, args...).Start()
}

// DisplaySpinnerASCII returns a simple ASCII spinner
func DisplaySpinnerASCII(step int) string {
	spinners := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	return spinners[step%len(spinners)]
}

// DisplayStatus displays a status message with optional spinner
func DisplayStatus(message string, showSpinner bool, step int) {
	if showSpinner {
		fmt.Printf("\r%s %s", DisplaySpinnerASCII(step), message)
	} else {
		fmt.Printf("\r%s", message)
	}
}

// ClearLine clears the current line in terminal
func ClearLine() {
	fmt.Print("\r\033[K")
}

// PrintSuccess prints a success message
func PrintSuccess(message string) {
	ClearLine()
	fmt.Printf("✓ %s\n", message)
}

// PrintError prints an error message
func PrintError(message string) {
	ClearLine()
	fmt.Printf("✗ %s\n", message)
}

// PrintInfo prints an info message
func PrintInfo(message string) {
	ClearLine()
	fmt.Printf("ℹ %s\n", message)
}

// PrintWarning prints a warning message
func PrintWarning(message string) {
	ClearLine()
	fmt.Printf("⚠ %s\n", message)
}

// TruncateString truncates a string to fit within max width
func TruncateString(s string, maxWidth int) string {
	if len(s) <= maxWidth {
		return s
	}
	// Try to truncate at word boundary
	if idx := strings.LastIndex(s[:maxWidth], " "); idx > 0 {
		return s[:idx] + "..."
	}
	return s[:maxWidth-3] + "..."
}
