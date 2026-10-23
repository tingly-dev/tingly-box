// @tingly/projects/transcript: an agent session's transcript, as a model and
// as the components that render it. Props only — the host (Projects'
// thread and session pages) loads the entries and answers requests; nothing here calls an
// API. Strings (projects.transcript.*) ship in ./locales; the host merges them
// into its i18n bundle (import @tingly/projects/transcript/locales/<lang>).

export type {
    ActivityStep, ApprovalSubject, AskOption, AskQuestion, RequestBlock, RequestOutcome, TaskEvent, TaskState, ThinkingStep, Todo, ToolStep, TranscriptAttachment, TranscriptBlock, TranscriptEntry, WorkingVerb,
} from './model';
export {
    agentReport, agentStatus, approvalSubject, askQuestions, blockTime, buildTranscript, firstNewBlock, foldTasks, formatTokens, isAgentTool, latestTodos, pendingRequestId, requestOutcome, toolSummary, workingState,
} from './model';

export type {TrajectoryAnchor, TrajectoryFilter, TrajectoryKind, TrajectoryRow, TrajectorySummary} from './trajectoryRows';
export {matchesFilter, plainLine, trajectoryRows, trajectorySummary} from './trajectoryRows';

export type {AgentSteps, AttachmentsComponent, CodeBlockComponent, DraftStorage, SubagentSteps} from './context';
export {createMemoryDraftStorage} from './context';

export {default as Transcript, SubagentFocus} from './Transcript';
export type {Respond} from './Transcript';
export {default as Trajectory} from './Trajectory';
export {default as Markdown} from './Markdown';
