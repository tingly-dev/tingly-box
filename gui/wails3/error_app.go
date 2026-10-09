package main

import (
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// noticeAction is an optional "do something about it" offer shown as a button
// next to Cancel in the native notice dialog.
type noticeAction struct {
	Prompt string // appended to the notice message as the question
	Label  string // confirm-button text
	// Run performs the action. It must run before the notice app quits: on some
	// platforms (notably macOS) app.Quit() terminates the process outright and
	// app.Run() never returns, so work scheduled "after Run" would be lost.
	Run func() error
}

// runErrorApp shows a port-unavailable notice.
func runErrorApp(message string) {
	_, _ = runNoticeApp("Port Unavailable", message, nil)
}

// runNoticeApp shows title/message in a single native dialog (no window) and
// blocks until it is dismissed. With a non-nil action the dialog also offers to
// confirm it; the return values report whether the user did (always false
// without an action) and the error from action.Run.
func runNoticeApp(title, message string, action *noticeAction) (confirmed bool, runErr error) {
	app := application.New(application.Options{
		Name:        AppName,
		Description: AppDescription,
		Mac: application.MacOptions{
			// There is no window at all, so "last window closed" would end the
			// process the moment the dialog is dismissed on macOS, killing the
			// goroutine that is still stopping the old server and relaunching.
			// Every path quits explicitly via app.Quit() instead.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		if action == nil {
			dialog := app.Dialog.Warning().SetTitle(title).SetMessage(message)
			dialog.AddButton("OK").SetAsDefault().OnClick(func() { app.Quit() })
			dialog.Show()
			return
		}
		dialog := app.Dialog.Question().SetTitle(title).SetMessage(message + "\n\n" + action.Prompt)
		dialog.AddButton(action.Label).SetAsDefault().OnClick(func() {
			log.Printf("Notice %q: %q confirmed", title, action.Label)
			confirmed = true
			// Off the UI thread: stopping the old server can take seconds.
			go func() {
				if action.Run != nil {
					runErr = action.Run()
				}
				app.Quit()
			}()
		})
		dialog.AddButton("Cancel").SetAsCancel().OnClick(func() {
			log.Printf("Notice %q: cancelled", title)
			app.Quit()
		})
		dialog.Show()
	})

	_ = app.Run()
	return confirmed, runErr
}
