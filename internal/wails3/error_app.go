package main

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// runErrorApp creates a minimal app with just an error message window
func runErrorApp(message string) {
	app := application.New(application.Options{
		Name:        AppName,
		Description: AppDescription,
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// Create window first (without URL)
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:  "error-window",
		Title: "Tingly Box - Error",
		Mac: application.MacWindow{
			Backdrop: application.MacBackdropTranslucent,
			TitleBar: application.MacTitleBarDefault,
		},
		BackgroundColour: application.NewRGB(241, 245, 249),
		Width:            500,
		Height:           500,
	})

	// Create HTML error page with message
	errorHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            background-color: #f1f5f9;
            color: #1e293b;
        }
        .container {
            text-align: center;
            padding: 48px;
            background: #ffffff;
            border-radius: 16px;
            box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);
            max-width: 400px;
        }
        .error-icon {
            font-size: 56px;
            margin-bottom: 16px;
        }
        h1 {
            color: #dc2626;
            font-size: 24px;
            font-weight: 600;
            margin-bottom: 12px;
        }
        p {
            font-size: 15px;
            line-height: 1.6;
            color: #64748b;
            white-space: pre-line;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="error-icon">⚠️</div>
        <h1>Port Unavailable</h1>
        <p>%s</p>
    </div>
</body>
</html>`, message)

	// Set HTML content directly
	window.SetHTML(errorHTML)

	// Run the error app
	_ = app.Run()
}
