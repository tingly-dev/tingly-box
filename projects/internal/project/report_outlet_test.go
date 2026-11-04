package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/llm"
)

func finishedReport(th Report, kind, text string) Report {
	th.Kind, th.Text = kind, text
	return th
}

func TestFinishedReportReachesTheChatOnlyThroughTheCoordinator(t *testing.T) {
	f, svc, sink := channelFixture(t)
	th := startWorker(t, f)
	waitQuiet(t, f, svc, th.ID)
	_, err := svc.BindChannel("telegram", "1", "", "", f.project.ID)
	require.NoError(t, err)

	svc.Coordinator.onReport(finishedReport(Report{Thread: *th}, ReportFinished, "Opened PR #7."))
	got := sink.next(t).text
	assert.Contains(t, got, "Echo: Opened PR #7.", "the coordinator's reply is the one message")
	assert.NotContains(t, got, "Finished:")
	sink.none(t)
}

func TestFinishedReportIsSilentWhenTheCoordinatorHasNothingToAdd(t *testing.T) {
	f, svc, sink := channelFixtureReplying(t, func(turn *enginetest.Turn) error {
		turn.Say("ok", 1, 1)
		return nil
	}, func(llm.Request) llm.Result { return llm.Result{Text: noUpdate} })
	th := startWorker(t, f)
	waitQuiet(t, f, svc, th.ID)
	_, err := svc.BindChannel("telegram", "1", "", "", f.project.ID)
	require.NoError(t, err)

	svc.Coordinator.onReport(finishedReport(Report{Thread: *th}, ReportFinished, "Opened PR #7."))
	waitQuiet(t, f, svc, th.ID)
	sink.none(t)
}

func TestFailureAndQuestionStillReachTheChatBeforeTheCoordinator(t *testing.T) {
	f, svc, sink := channelFixture(t)
	th := startWorker(t, f)
	waitQuiet(t, f, svc, th.ID)
	_, err := svc.BindChannel("telegram", "1", "", "", f.project.ID)
	require.NoError(t, err)

	svc.Coordinator.onReport(finishedReport(Report{Thread: *th}, ReportFailed, "exit status 1"))
	assert.Contains(t, sink.next(t).text, "Failed: exit status 1")
	assert.Contains(t, sink.next(t).text, "Echo: exit status 1")

	svc.Coordinator.onReport(finishedReport(Report{Thread: *th}, ReportNeedsInput, "Keep the flag?"))
	assert.Contains(t, sink.next(t).text, "Needs you: Keep the flag?")
	assert.Contains(t, sink.next(t).text, "Echo: Keep the flag?")
}

func TestFinishedReportFallsBackToTheFoldedLineWhileThePauseLasts(t *testing.T) {
	f, svc, sink := channelFixture(t)
	th := startWorker(t, f)
	waitQuiet(t, f, svc, th.ID)
	_, err := svc.BindChannel("telegram", "1", "", "", f.project.ID)
	require.NoError(t, err)
	require.NoError(t, svc.SetPaused(f.project, true))

	svc.Coordinator.onReport(finishedReport(Report{Thread: *th}, ReportFinished, "Opened PR #7."))
	assert.Contains(t, sink.next(t).text, "Finished: Opened PR #7.", "nobody else would tell the user")
	sink.none(t)
}
