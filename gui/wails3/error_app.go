package main

import (
	"fmt"
	"html"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// noticeAction is an optional "do something about it" offer shown as a native
// question dialog on top of the notice window.
type noticeAction struct {
	Prompt string // dialog body
	Label  string // confirm-button text
	// Run performs the action. It must run before the notice app quits: on some
	// platforms (notably macOS) app.Quit() terminates the process outright and
	// app.Run() never returns, so work scheduled "after Run" would be lost.
	Run func() error
}

// runErrorApp creates a minimal app with just an error message window
func runErrorApp(message string) {
	_, _ = runNoticeApp("Port Unavailable", message, nil)
}

// runNoticeApp shows title/message in a minimal window and blocks until it is
// closed. With a non-nil action it also asks the user to confirm it; the
// return value reports whether they did (always false without an action) and
// the error from action.Run.
func runNoticeApp(title, message string, action *noticeAction) (confirmed bool, runErr error) {
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
        <h1>%s</h1>
        <p>%s</p>
    </div>
</body>
</html>`, html.EscapeString(title), html.EscapeString(message))

	// Set HTML content directly
	window.SetHTML(errorHTML)

	if action != nil {
		app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
			dialog := app.Dialog.Question().SetTitle(title).SetMessage(action.Prompt)
			dialog.AddButton(action.Label).SetAsDefault().OnClick(func() {
				confirmed = true
				// Off the UI thread: stopping the old server can take seconds.
				go func() {
					if action.Run != nil {
						runErr = action.Run()
					}
					app.Quit()
				}()
			})
			dialog.AddButton("Cancel").SetAsCancel().OnClick(func() { app.Quit() })
			dialog.Show()
		})
	}

	// Run the error app
	_ = app.Run()
	return confirmed, runErr
}
