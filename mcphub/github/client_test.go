package github

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedact(t *testing.T) {
	for in, want := range map[string]string{
		"token ghp_abcdefghijklmnopqrstuvwxyz0123456789 end": "token [redacted] end",
		"github_pat_11ABCDEFG0abcdefghijklmnopqrstuvwxyz":    "[redacted]",
		"Authorization: Bearer abc.def-ghi":                  "Authorization: Bearer [redacted]",
		"GH_TOKEN=abc123 gh":                                 "GH_TOKEN=[redacted] gh",
		"fatal: https://user:p4ss@github.com/a/b.git":        "fatal: https://github.com/a/b.git",
		"https://x-access-token:ghs_abc@github.com/a/b":      "https://github.com/a/b",
		"nothing secret here":                                "nothing secret here",
	} {
		assert.Equal(t, want, Redact(in), in)
	}
}

func TestParseRemote(t *testing.T) {
	for remote, want := range map[string]string{
		"https://github.com/acme/widgets.git":                           "acme/widgets",
		"https://github.com/acme/widgets":                               "acme/widgets",
		"git@github.com:acme/widgets.git":                               "acme/widgets",
		"ssh://git@github.com/acme/widgets.git":                         "acme/widgets",
		"https://x-access-token:ghs_secret@github.com/acme/widgets.git": "acme/widgets",
		"https://gitlab.com/acme/widgets.git":                           "",
		"https://github.com/acme":                                       "",
		"/some/local/path":                                              "",
		"https://github.com/acme/widgets/extra.git":                     "",
	} {
		r, err := ParseRemote(remote)
		if want == "" {
			assert.Error(t, err, remote)
			continue
		}
		require.NoError(t, err, remote)
		assert.Equal(t, want, r.Slug(), remote)
	}
}

// fakeGH writes a gh executable that records its arguments and body and
// answers from a canned file, so Client is exercised with no network.
func fakeGH(t *testing.T, answer string, exit int) (bin, logFile string) {
	t.Helper()
	dir := t.TempDir()
	logFile = filepath.Join(dir, "log")
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> " + logFile + "\n" +
		"cat >> " + logFile + "\n" +
		"if [ " + string(rune('0'+exit)) + " -ne 0 ]; then echo 'gh: HTTP 422 Unprocessable ghp_abcdefghijklmnopqrstuvwxyz0123456789' >&2; exit " + string(rune('0'+exit)) + "; fi\n" +
		"cat <<'EOF'\n" + answer + "\nEOF\n"
	bin = filepath.Join(dir, "gh")
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	return bin, logFile
}

// ghGraphQL writes a gh executable that answers a read with the given JSON.
func ghGraphQL(t *testing.T, answer string) (bin, logFile string) {
	t.Helper()
	return fakeGH(t, answer, 0)
}

func TestClientUsesRESTWithAJSONBody(t *testing.T) {
	bin, logFile := fakeGH(t, `{"number":7,"html_url":"https://github.com/acme/widgets/pull/7","title":"T","state":"open","head":{"ref":"tp/x","sha":"abc"},"base":{"ref":"main"},"mergeable_state":"dirty"}`, 0)
	g := Client{Bin: bin}
	repo := Repo{Owner: "acme", Name: "widgets"}

	title, body := "New \"title\"", "line 1\nline 2 $(rm -rf /) `x`"
	it, err := g.UpdatePR(context.Background(), repo, 7, &title, &body, nil)
	require.NoError(t, err)
	assert.Equal(t, 7, it.Number)
	assert.Equal(t, "conflicts", it.MergeState)
	b, err := os.ReadFile(logFile)
	require.NoError(t, err)
	got := string(b)
	assert.Contains(t, got, "api -X PATCH --input - repos/acme/widgets/pulls/7", "REST, not `gh pr edit`")
	assert.Contains(t, got, `"title":"New \"title\""`)
	assert.Contains(t, got, `$(rm -rf /)`, "the body travels as data, never through a shell")
	assert.NotContains(t, got, `"base"`, "an unset field is not sent")

	// An issue is edited through the issues endpoint, with only the fields given.
	_, err = g.UpdateIssue(context.Background(), repo, 9, &title, nil)
	require.NoError(t, err)
	b, _ = os.ReadFile(logFile)
	assert.Contains(t, string(b), "api -X PATCH --input - repos/acme/widgets/issues/9")

	_, err = g.CreatePR(context.Background(), repo, "T", "B", "tp/x", "main", true)
	require.NoError(t, err)
	b, _ = os.ReadFile(logFile)
	assert.Contains(t, string(b), "api -X POST --input - repos/acme/widgets/pulls")
	assert.Contains(t, string(b), `"draft":true`)
}

func TestClientErrorsAreRedactedAndGhMissingIsSaid(t *testing.T) {
	bin, _ := fakeGH(t, "", 1)
	_, err := Client{Bin: bin}.CreateIssue(context.Background(), Repo{Owner: "a", Name: "b"}, "t", "b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 422")
	assert.NotContains(t, err.Error(), "ghp_abcdef")

	_, err = Client{Bin: filepath.Join(t.TempDir(), "no-gh")}.DefaultBranch(context.Background(), Repo{Owner: "a", Name: "b"})
	assert.ErrorIs(t, err, ErrUnavailable)
}

func TestClientReadsAPRInOneRequestAndFoldsItsChecks(t *testing.T) {
	bin, logFile := ghGraphQL(t, `{"data":{"rateLimit":{"remaining":4321,"resetAt":"2026-11-15T09:00:00Z"},"repository":{"issueOrPullRequest":{
 "__typename":"PullRequest","number":7,"url":"https://github.com/acme/widgets/pull/7","title":"T","body":"B","state":"OPEN","isDraft":false,
 "headRefName":"tp/x","baseRefName":"main","mergeable":"MERGEABLE","mergeStateStatus":"BLOCKED",
 "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{"totalCount":5,"nodes":[
  {"__typename":"CheckRun","name":"build","status":"COMPLETED","conclusion":"SUCCESS","detailsUrl":"https://x/1"},
  {"__typename":"CheckRun","name":"docs","status":"COMPLETED","conclusion":"SKIPPED","detailsUrl":"https://x/2"},
  {"__typename":"CheckRun","name":"lint","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":"https://x/3"},
  {"__typename":"CheckRun","name":"e2e","status":"IN_PROGRESS","conclusion":null,"detailsUrl":"https://x/4"},
  {"__typename":"StatusContext","context":"ci/legacy","state":"ERROR","targetUrl":"https://x/5"}]}}}}]}}}}}`)
	it, err := Client{Bin: bin}.Read(context.Background(), Repo{Owner: "acme", Name: "widgets"}, 7)
	require.NoError(t, err)

	assert.True(t, it.IsPR)
	assert.Equal(t, "open", it.State)
	assert.Equal(t, "blocked", it.MergeState)
	assert.Equal(t, "tp/x", it.Head)
	assert.Equal(t, "failing", it.CI)
	assert.Equal(t, 4321, it.Rate.Remaining)
	assert.Equal(t, time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC), it.Rate.ResetAt)
	require.NotNil(t, it.Checks)
	assert.Equal(t, 2, it.Checks.Passing, "a skipped check is not a failure")
	assert.Equal(t, 2, it.Checks.Failing)
	assert.Equal(t, 1, it.Checks.Pending)
	assert.Equal(t, []Check{{Name: "lint", URL: "https://x/3"}, {Name: "ci/legacy", URL: "https://x/5"}}, it.Checks.Failed, "only what failed is named")

	b, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(b), "api -X POST --input - graphql"), "one request for the whole reading")
	assert.Contains(t, string(b), `"number":7`)
	assert.Contains(t, string(b), `"owner":"acme"`)
}

func TestClientReadsAnIssueAndAMergedPRWithoutChecks(t *testing.T) {
	bin, _ := ghGraphQL(t, `{"data":{"rateLimit":{"remaining":10,"resetAt":"2026-11-15T09:00:00Z"},"repository":{"issueOrPullRequest":{"__typename":"Issue","number":3,"url":"https://github.com/acme/widgets/issues/3","title":"I","body":"","state":"CLOSED"}}}}`)
	it, err := Client{Bin: bin}.Read(context.Background(), Repo{Owner: "acme", Name: "widgets"}, 3)
	require.NoError(t, err)
	assert.False(t, it.IsPR)
	assert.Equal(t, "closed", it.State)
	assert.Empty(t, it.CI)

	bin, _ = ghGraphQL(t, `{"data":{"rateLimit":{"remaining":10,"resetAt":"2026-11-15T09:00:00Z"},"repository":{"issueOrPullRequest":{"__typename":"PullRequest","number":4,"url":"u","title":"P","body":"","state":"MERGED","isDraft":false,"headRefName":"a","baseRefName":"main","mergeable":"UNKNOWN","mergeStateStatus":"UNKNOWN","commits":{"nodes":[]}}}}}`)
	it, err = Client{Bin: bin}.Read(context.Background(), Repo{Owner: "acme", Name: "widgets"}, 4)
	require.NoError(t, err)
	assert.Equal(t, "merged", it.State)
	assert.Empty(t, it.MergeState, "a merged PR has no merge state")
	assert.Empty(t, it.CI)
}

func TestClientReadOfAMissingNumberSaysSo(t *testing.T) {
	bin, _ := ghGraphQL(t, `{"data":{"rateLimit":{"remaining":10,"resetAt":"2026-11-15T09:00:00Z"},"repository":{"issueOrPullRequest":null}}}`)
	_, err := Client{Bin: bin}.Read(context.Background(), Repo{Owner: "acme", Name: "widgets"}, 99)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no issue or PR #99")
}

func TestChecksNameAtMostFiveFailuresAndCountTheRest(t *testing.T) {
	var nodes []string
	for i := 0; i < 8; i++ {
		nodes = append(nodes, `{"__typename":"CheckRun","name":"job`+string(rune('a'+i))+`","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":""}`)
	}
	bin, _ := ghGraphQL(t, `{"data":{"rateLimit":{"remaining":10,"resetAt":"2026-11-15T09:00:00Z"},"repository":{"issueOrPullRequest":{"__typename":"PullRequest","number":7,"url":"u","title":"P","body":"","state":"OPEN","headRefName":"a","baseRefName":"main","mergeStateStatus":"CLEAN","commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{"totalCount":8,"nodes":[`+strings.Join(nodes, ",")+`]}}}}]}}}}}`)
	it, err := Client{Bin: bin}.Read(context.Background(), Repo{Owner: "a", Name: "b"}, 7)
	require.NoError(t, err)
	assert.Len(t, it.Checks.Failed, ChecksListed)
	assert.Equal(t, 3, it.Checks.More)
	assert.Equal(t, 8, it.Checks.Failing)
}

func TestAFailureBeyondTheHundredChecksReadStillShows(t *testing.T) {
	bin, _ := ghGraphQL(t, `{"data":{"rateLimit":{"remaining":10,"resetAt":"2026-11-15T09:00:00Z"},"repository":{"issueOrPullRequest":{"__typename":"PullRequest","number":7,"url":"u","title":"P","body":"","state":"OPEN","headRefName":"a","baseRefName":"main","mergeStateStatus":"CLEAN","commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{"totalCount":130,"nodes":[{"__typename":"CheckRun","name":"ok","status":"COMPLETED","conclusion":"SUCCESS","detailsUrl":""}]}}}}]}}}}}`)
	it, err := Client{Bin: bin}.Read(context.Background(), Repo{Owner: "a", Name: "b"}, 7)
	require.NoError(t, err)
	assert.Equal(t, "failing", it.CI, "the rollup has the last word when not every check was read")
}

func TestClientProtectedReadsTheBranchAndTreatsAMissingOneAsUnprotected(t *testing.T) {
	repo := Repo{Owner: "acme", Name: "widgets"}
	bin, logFile := fakeGH(t, `{"name":"release/1.0","protected":true}`, 0)
	got, err := Client{Bin: bin}.Protected(context.Background(), repo, "release/1.0")
	require.NoError(t, err)
	assert.True(t, got)
	b, _ := os.ReadFile(logFile)
	assert.Contains(t, string(b), "api -X GET repos/acme/widgets/branches/release/1.0")

	// A branch only this checkout has is not on GitHub: 404, not protected.
	dir := t.TempDir()
	notFound := filepath.Join(dir, "gh")
	require.NoError(t, os.WriteFile(notFound, []byte("#!/bin/sh\necho 'gh: Branch not found (HTTP 404)' >&2\nexit 1\n"), 0o755))
	got, err = Client{Bin: notFound}.Protected(context.Background(), repo, "feat/local")
	require.NoError(t, err)
	assert.False(t, got)

	// Any other failure is an error: an unreadable answer must not read as "not protected".
	bin, _ = fakeGH(t, "", 1)
	_, err = Client{Bin: bin}.Protected(context.Background(), repo, "feat/x")
	assert.Error(t, err)
}

func TestClientSquashPRPutsAPinnedSquashMerge(t *testing.T) {
	repo := Repo{Owner: "acme", Name: "widgets"}
	bin, logFile := fakeGH(t, `{"merged":true,"message":"Pull Request successfully merged","sha":"def"}`, 0)
	require.NoError(t, Client{Bin: bin}.SquashPR(context.Background(), repo, 7, "abc123", "Add a", "Body."))
	b, err := os.ReadFile(logFile)
	require.NoError(t, err)
	got := string(b)
	assert.Contains(t, got, "api -X PUT --input - repos/acme/widgets/pulls/7/merge")
	assert.Contains(t, got, `"merge_method":"squash"`)
	assert.Contains(t, got, `"sha":"abc123"`, "only the commit that was pushed is merged")
	assert.Contains(t, got, `"commit_title":"Add a"`)

	bin, logFile = fakeGH(t, `{"merged":true}`, 0)
	require.NoError(t, Client{Bin: bin}.SquashPR(context.Background(), repo, 7, "abc123", "", ""))
	b, _ = os.ReadFile(logFile)
	assert.NotContains(t, string(b), "commit_title", "no message: GitHub's default")

	// GitHub answering "not merged" is an error, whatever the status.
	bin, _ = fakeGH(t, `{"merged":false,"message":"Pull Request is not mergeable"}`, 0)
	err = Client{Bin: bin}.SquashPR(context.Background(), repo, 7, "abc123", "", "")
	assert.ErrorContains(t, err, "not mergeable")

	bin, _ = fakeGH(t, "", 1)
	assert.Error(t, Client{Bin: bin}.SquashPR(context.Background(), repo, 7, "abc123", "", ""))
}
