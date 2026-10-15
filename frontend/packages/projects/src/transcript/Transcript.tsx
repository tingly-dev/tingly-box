import {Block, Build, Cancel, Check, CheckCircle, ChevronRight, Close, ErrorOutline, ExpandMore, Help, Psychology, Robot, Terminal} from '@tingly/ui/icons';
import {Alert, Box, Button, Chip, CircularProgress, Collapse, Paper, Stack, TextField, Typography} from '@mui/material';
import type {Theme} from '@mui/material/styles';
import {getReadableAccent} from '@tingly/ui/theme/status';
import {fontMono, fontSizes} from '@tingly/ui/theme/fonts';
import type {TFunction} from 'i18next';
import type {ReactNode} from 'react';
import {useState} from 'react';
import {useTranslation} from 'react-i18next';
import type {CodeBlockComponent, DraftStorage} from './context';
import {TranscriptHostProvider, useDraft, useTranscriptHost} from './context';
import type {ActivityStep, RequestBlock, RequestOutcome, TaskState, ThinkingStep, ToolStep, TranscriptBlock, TranscriptEntry} from './model';
import {agentReport, agentStatus, approvalSubject, formatTokens, requestOutcome, toolSummary} from './model';
import Markdown from './Markdown';

// Respond answers a request. remember asks to allow the tool from now on
// without asking (offered only when the host passes rememberLabel).
export type Respond = (requestId: string, approved: boolean, answer: string, remember?: boolean) => Promise<boolean>;

interface TranscriptProps {
    blocks: TranscriptBlock[];
    pendingRequestId?: string;
    working: boolean;
    // Opens every tool call, thought and subagent; each can still be toggled
    // on its own.
    expandAll?: boolean;
    onRespond: Respond;
    // Where request-card drafts live; in memory unless the host passes a store.
    drafts?: DraftStorage;
    // Renders fenced code in replies; plain monospace unless the host passes one.
    codeBlock?: CodeBlockComponent;
    // The label of a second "allow" on an approval card that also allows the
    // tool from now on ("Allow Bash in this thread"); none unless passed.
    rememberLabel?: (tool: string) => string;
}

const mono = {fontFamily: fontMono, fontSize: fontSizes.md};

// The theme's body variants default to the secondary text color, which suits
// metadata. The conversation itself is the subject of this page, so it uses
// the primary color at reading size; labels, notes and tool detail stay
// secondary so they recede.
const conversationText = {
    color: 'text.primary', fontSize: '0.875rem', lineHeight: 1.65, whiteSpace: 'pre-wrap', wordBreak: 'break-word',
} as const;

const preBox = {
    ...mono, m: 0, p: 1, borderRadius: 1, maxHeight: 220, overflow: 'auto',
    whiteSpace: 'pre-wrap', wordBreak: 'break-word', bgcolor: 'action.hover', color: 'text.secondary',
} as const;

const UserBubble = ({message}: {message: TranscriptEntry}) => (
    <Box sx={{display: 'flex', flexDirection: 'column', alignItems: 'flex-end'}}>
        {message.author && <Typography variant="caption" sx={{color: 'text.secondary', mb: 0.25}}>{message.author}</Typography>}
        <Box sx={{maxWidth: '80%', bgcolor: 'action.hover', px: 1.75, py: 1, borderRadius: 1.5}}>
            <Typography variant="body1" sx={conversationText}>{message.content}</Typography>
        </Box>
    </Box>
);

const AssistantText = ({message}: {message: TranscriptEntry}) => <Markdown content={message.content}/>;

const formatDuration = (ms: number): string => {
    const sec = Math.round(ms / 1000);
    return sec < 60 ? `${sec}s` : `${Math.floor(sec / 60)}m ${sec % 60}s`;
};

const prettyInput = (input: unknown): string => (input == null ? '' : JSON.stringify(input, null, 2));

const statusLabel = (t: TFunction, status: TaskState['status']) => ({
    running: t('projects.transcript.taskRunning', {defaultValue: 'running'}),
    completed: t('projects.transcript.taskDone', {defaultValue: 'done'}),
    stopped: t('projects.transcript.taskStopped', {defaultValue: 'stopped'}),
    failed: t('projects.transcript.taskFailed', {defaultValue: 'failed'}),
}[status]);

// useExpanded is a row's open state: it follows "expand all", and a row's own
// toggle holds until "expand all" is flipped again.
const useExpanded = (expandAll: boolean): [boolean, () => void] => {
    const [toggled, setToggled] = useState<{under: boolean; open: boolean} | null>(null);
    const open = toggled?.under === expandAll ? toggled.open : expandAll;
    return [open, () => setToggled({under: expandAll, open: !open})];
};

// TaskTag marks a call that runs in the background, with its state: the
// turn that started it may be long over while it still works.
const TaskTag = ({task}: {task: TaskState}) => {
    const {t} = useTranslation();
    return (
        <Chip
            size="small"
            variant="outlined"
            color={task.status === 'failed' ? 'error' : 'default'}
            icon={task.status === 'running' ? <CircularProgress size={10} sx={{ml: '6px !important'}}/> : undefined}
            label={`${t('projects.transcript.background', {defaultValue: 'background'})} · ${statusLabel(t, task.status)}`}
            sx={{height: 20, flexShrink: 0, color: (theme) => task.status === 'failed' ? getReadableAccent(theme, 'error') : undefined, borderColor: (theme) => task.status === 'failed' ? getReadableAccent(theme, 'error') : undefined, '& .MuiChip-label': {px: 0.75, fontSize: fontSizes.xs}}}
        />
    );
};

const outcomeColor = (theme: Theme, outcome: RequestOutcome): string => {
    switch (outcome) {
        case 'allowed':
        case 'answered':
            return getReadableAccent(theme, 'success');
        case 'denied':
        case 'declined':
            return getReadableAccent(theme, 'error');
        default:
            return theme.palette.text.disabled;
    }
};

// OutcomeTag says how a settled request ended, as a small tag at the end of
// the line it belongs to.
const OutcomeTag = ({outcome}: {outcome: RequestOutcome}) => {
    const {t} = useTranslation();
    const ok = outcome === 'allowed' || outcome === 'answered';
    const label = {
        pending: '',
        allowed: t('projects.transcript.outcomeAllowed', {defaultValue: 'Allowed'}),
        answered: t('projects.transcript.outcomeAnswered', {defaultValue: 'Answered'}),
        denied: t('projects.transcript.outcomeDenied', {defaultValue: 'Denied'}),
        declined: t('projects.transcript.outcomeDeclined', {defaultValue: 'Declined'}),
        unanswered: t('projects.transcript.outcomeNotAnswered', {defaultValue: 'Not answered'}),
    }[outcome];
    return (
        <Box
            component="span"
            data-outcome={outcome}
            sx={{display: 'inline-flex', alignItems: 'center', gap: 0.25, flexShrink: 0, fontSize: fontSizes.sm, color: (theme) => outcomeColor(theme, outcome)}}
        >
            {ok ? <Check sx={{fontSize: 13}}/> : <Block sx={{fontSize: 13}}/>}
            {label}
        </Box>
    );
};

// A quiet, clickable line: what a tool call or a thought is, one click away
// from its detail. The reply stays the visual anchor.
const RowLine = ({open, onToggle, children, error = false}: {open: boolean; onToggle: () => void; children: ReactNode; error?: boolean}) => (
    <Box
        role="button"
        tabIndex={0}
        aria-expanded={open}
        onClick={onToggle}
        onKeyDown={(event) => {
            if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault();
                onToggle();
            }
        }}
        sx={{
            display: 'flex', alignItems: 'center', gap: 0.75, minWidth: 0, py: 0.25, px: 0.5, mx: -0.5, borderRadius: 1,
            cursor: 'default', userSelect: 'none',
            color: (theme) => error ? getReadableAccent(theme, 'error') : theme.palette.text.secondary,
            '&:hover': {bgcolor: 'action.hover', color: (theme) => error ? getReadableAccent(theme, 'error') : theme.palette.text.primary},
        }}
    >
        {children}
        {open ? <ExpandMore sx={{fontSize: 16, flexShrink: 0}}/> : <ChevronRight sx={{fontSize: 16, flexShrink: 0}}/>}
    </Box>
);

const Section = ({label, children}: {label: string; children: string}) => (
    <Box>
        <Typography variant="caption" sx={{color: 'text.secondary'}}>{label}</Typography>
        <Box component="pre" sx={preBox}>{children}</Box>
    </Box>
);

// ToolRow is one tool call on one line: the tool and the concrete thing it
// did (the command, the file), how its approval went when it needed one, and
// whether it failed. Opened, it shows its input and its output.
const ToolRow = ({step, live, expandAll, pendingRequestId}: {step: ToolStep; live: boolean; expandAll: boolean; pendingRequestId?: string}) => {
    const {t} = useTranslation();
    const [open, toggle] = useExpanded(expandAll);
    const summary = toolSummary(step.input);
    const request = step.request;
    const requestId = request?.message.request_id;
    // The pending approval is its own card below the call.
    const outcome = request && !(requestId && requestId === pendingRequestId) ? requestOutcome(request, false) : undefined;
    // A denied call fails by definition; the tag already says why.
    const failed = step.isError && outcome !== 'denied';
    const input = prettyInput(step.input);

    return (
        <Box data-call-ids={step.id || undefined} data-request-id={outcome ? requestId : undefined}>
            <RowLine open={open} onToggle={toggle} error={failed}>
                {live
                    ? <CircularProgress size={12} sx={{flexShrink: 0, mx: 0.125}}/>
                    : failed ? <ErrorOutline sx={{fontSize: 14, flexShrink: 0}}/> : <Build sx={{fontSize: 14, flexShrink: 0}}/>}
                <Typography variant="body2" sx={{fontWeight: 600, flexShrink: 0, color: 'inherit'}}>
                    {step.name || t('projects.transcript.result', {defaultValue: 'Result'})}
                </Typography>
                <Typography variant="body2" noWrap sx={{...mono, flex: 1, minWidth: 0, color: 'inherit'}}>{summary}</Typography>
                {step.task?.background && <TaskTag task={step.task}/>}
                {outcome && <OutcomeTag outcome={outcome}/>}
                {failed && (
                    <Typography variant="body2" sx={{flexShrink: 0, color: 'inherit'}}>
                        {outcome ? '· ' : ''}{t('projects.transcript.callFailed', {defaultValue: 'failed'})}
                    </Typography>
                )}
            </RowLine>
            <Collapse in={open} unmountOnExit>
                <Stack spacing={0.75} sx={{mt: 0.5, mb: 1, pl: 1.5, borderLeft: 2, borderColor: 'divider'}}>
                    {input && <Section label={t('projects.transcript.input', {defaultValue: 'Input'})}>{input}</Section>}
                    {step.result && <Section label={t('projects.transcript.output', {defaultValue: 'Output'})}>{step.result}</Section>}
                </Stack>
            </Collapse>
        </Box>
    );
};

// ThoughtRow folds a run of thinking into one quiet "Thought" line.
const ThoughtRow = ({steps, live, expandAll}: {steps: ThinkingStep[]; live: boolean; expandAll: boolean}) => {
    const {t} = useTranslation();
    const [open, toggle] = useExpanded(expandAll);
    return (
        <Box>
            <RowLine open={open} onToggle={toggle}>
                {live ? <CircularProgress size={12} sx={{flexShrink: 0, mx: 0.125}}/> : <Psychology sx={{fontSize: 14, flexShrink: 0}}/>}
                <Typography variant="body2" sx={{color: 'inherit'}}>{t('projects.transcript.thought', {defaultValue: 'Thought'})}</Typography>
            </RowLine>
            <Collapse in={open} unmountOnExit>
                <Stack spacing={1} sx={{mt: 0.5, mb: 1, pl: 1.5, borderLeft: 2, borderColor: 'divider'}}>
                    {steps.map((s, i) => (
                        <Typography key={i} variant="body2" color="text.secondary" sx={{fontStyle: 'italic', whiteSpace: 'pre-wrap'}}>{s.text}</Typography>
                    ))}
                </Stack>
            </Collapse>
        </Box>
    );
};

// ActivityRun is a run of thinking and tool calls: one line per call, and
// each run of thinking folded into one "Thought" line. While the turn runs,
// the last line carries the spinner.
const ActivityRun = ({steps, live, expandAll, pendingRequestId}: {steps: ActivityStep[]; live: boolean; expandAll: boolean; pendingRequestId?: string}) => {
    const rows: (ToolStep | ThinkingStep[])[] = [];
    for (const s of steps) {
        if (s.type === 'tool') rows.push(s);
        else if (Array.isArray(rows[rows.length - 1])) (rows[rows.length - 1] as ThinkingStep[]).push(s);
        else rows.push([s]);
    }
    return (
        <Stack spacing={0.25}>
            {rows.map((r, i) => {
                const last = live && i === rows.length - 1;
                return Array.isArray(r)
                    ? <ThoughtRow key={i} steps={r} live={last} expandAll={expandAll}/>
                    : <ToolRow key={r.id || i} step={r} live={last} expandAll={expandAll} pendingRequestId={pendingRequestId}/>;
            })}
        </Stack>
    );
};

interface ApprovalDraft {answer: string; status: 'idle' | 'sending' | 'accepted'}
const decodeApproval = (value: unknown): ApprovalDraft => {
    const saved = value && typeof value === 'object' ? value as Record<string, unknown> : {};
    return {answer: typeof saved.answer === 'string' ? saved.answer : '', status: saved.status === 'accepted' ? 'accepted' : 'idle'};
};

// Subject is the concrete thing an approval is about: a command whole, in a
// code block; a path or URL as the literal value.
const Subject = ({tool, input, inline = false}: {tool: string; input: unknown; inline?: boolean}) => {
    const subject = approvalSubject(tool, input);
    if (!subject.text) return null;
    if (inline) {
        return <Typography component="span" variant="body2" noWrap sx={{...mono, minWidth: 0, flex: 1, color: 'inherit'}}>{subject.text.split('\n')[0]}</Typography>;
    }
    return (
        <Box
            component="pre"
            data-subject={subject.kind}
            sx={{...mono, m: 0, p: 1, borderRadius: 1, bgcolor: 'action.hover', color: 'text.primary', whiteSpace: 'pre-wrap', wordBreak: 'break-word', maxHeight: 200, overflow: 'auto'}}
        >
            {subject.text}
        </Box>
    );
};

// SettledRequest is a request nothing waits on any more, as one quiet line:
// what was asked and how it ended. An answered question reads as the
// question and, under it, the answer.
const SettledRequest = ({block}: {block: RequestBlock}) => {
    const {t} = useTranslation();
    const {message, response} = block;
    const isAsk = message.kind === 'ask_request';
    const outcome = requestOutcome(block, false);
    return (
        <Stack data-request-id={message.request_id} direction="row" spacing={0.75} sx={{alignItems: 'flex-start', color: 'text.secondary', minWidth: 0}}>
            {isAsk ? <Help sx={{fontSize: 14, mt: 0.375, flexShrink: 0}}/> : <Terminal sx={{fontSize: 14, mt: 0.375, flexShrink: 0}}/>}
            <Box sx={{flex: 1, minWidth: 0}}>
                <Stack direction="row" spacing={0.75} sx={{alignItems: 'baseline', minWidth: 0}}>
                    {isAsk
                        ? <Typography variant="body2" sx={{color: 'inherit', wordBreak: 'break-word'}}>{message.content}</Typography>
                        : (
                            <>
                                <Typography variant="body2" sx={{fontWeight: 600, flexShrink: 0, color: 'inherit'}}>{message.content}</Typography>
                                <Subject tool={message.content} input={message.payload} inline/>
                            </>
                        )}
                </Stack>
                {isAsk && response && (
                    <Typography variant="body2" sx={{color: outcome === 'answered' ? 'text.primary' : 'text.secondary', whiteSpace: 'pre-wrap', wordBreak: 'break-word'}}>
                        ↳ {response.content.trim() || t('projects.transcript.noAnswer', {defaultValue: '(no answer)'})}
                    </Typography>
                )}
            </Box>
            {!(isAsk && response) && <OutcomeTag outcome={outcome}/>}
        </Stack>
    );
};

// RequestCard is the approval or question the live turn waits on: it shows
// what exactly is being approved, with the full input one click away.
const RequestCard = ({block, onRespond}: {
    block: RequestBlock;
    onRespond: (approved: boolean, answer: string, remember?: boolean) => Promise<boolean>;
}) => {
    const {t} = useTranslation();
    const {rememberLabel} = useTranscriptHost();
    const {message} = block;
    const [request, setRequest, latestRequest] = useDraft(`requestDraft:${message.request_id ?? ''}`, decodeApproval);
    const [details, setDetails] = useState(false);
    const answer = request.answer;
    const setAnswer = (text: string) => setRequest((previous) => ({...previous, answer: text}));
    const busy = request.status === 'sending';
    const submitted = request.status === 'accepted';
    const [failed, setFailed] = useState(false);
    const isAsk = message.kind === 'ask_request';
    const input = isAsk ? '' : prettyInput(message.payload);

    const respond = async (approved: boolean, remember?: boolean) => {
        if (latestRequest().status !== 'idle' || (approved && isAsk && !answer.trim())) return;
        setRequest((previous) => ({...previous, status: 'sending'}));
        setFailed(false);
        try {
            if (await (remember ? onRespond(approved, answer.trim(), true) : onRespond(approved, answer.trim()))) {
                setRequest((previous) => ({answer: previous.answer === answer ? '' : previous.answer, status: 'accepted'}));
            } else setFailed(true);
        } catch {
            setFailed(true);
        } finally {
            setRequest((previous) => previous.status === 'sending' ? {...previous, status: 'idle'} : previous);
        }
    };

    return (
        <Paper data-request-id={message.request_id} variant="outlined" sx={{p: 1.5, borderRadius: 1.5, borderColor: 'primary.main'}}>
            <Stack direction="row" spacing={1} sx={{alignItems: 'center', mb: 1}}>
                {isAsk
                    ? <Help sx={{fontSize: 16, color: 'primary.main'}}/>
                    : <Terminal sx={{fontSize: 16, color: 'primary.main'}}/>}
                <Typography variant="body2" sx={{fontWeight: 600, color: 'text.primary', fontSize: '0.875rem', wordBreak: 'break-word'}}>
                    {isAsk
                        ? message.content
                        : t('projects.transcript.approvalTitle', {defaultValue: 'Allow {{tool}}?', tool: message.content})}
                </Typography>
            </Stack>
            {!isAsk && (
                <Box sx={{mb: 1}}>
                    <Subject tool={message.content} input={message.payload}/>
                    {input && (
                        <>
                            <Box
                                component="button"
                                type="button"
                                aria-expanded={details}
                                onClick={() => setDetails((v) => !v)}
                                sx={{
                                    display: 'inline-flex', alignItems: 'center', gap: 0.25, mt: 0.5, p: 0, border: 0, bgcolor: 'transparent',
                                    font: 'inherit', fontSize: fontSizes.sm, color: 'text.secondary', cursor: 'pointer', '&:hover': {color: 'text.primary'},
                                }}
                            >
                                {t('projects.transcript.details', {defaultValue: 'Details'})}
                                {details ? <ExpandMore sx={{fontSize: 14}}/> : <ChevronRight sx={{fontSize: 14}}/>}
                            </Box>
                            <Collapse in={details} unmountOnExit>
                                <Box component="pre" sx={{...preBox, mt: 0.5}}>{input}</Box>
                            </Collapse>
                        </>
                    )}
                </Box>
            )}
            {isAsk && (
                <TextField
                    size="small"
                    fullWidth
                    multiline
                    maxRows={8}
                    disabled={busy || submitted}
                    slotProps={{htmlInput: {'aria-label': t('projects.transcript.answerPlaceholder', {defaultValue: 'Your answer'})}}}
                    onKeyDown={(event) => {
                        if (event.key === 'Enter' && (event.ctrlKey || event.metaKey) && !event.nativeEvent.isComposing) {
                            event.preventDefault();
                            void respond(true);
                        }
                    }}
                    placeholder={t('projects.transcript.answerPlaceholder', {defaultValue: 'Your answer'})}
                    value={answer}
                    onChange={(e) => setAnswer(e.target.value)}
                    sx={{mb: 1}}
                />
            )}
            {failed && <Alert severity="error" sx={{mb: 1}}>{t('projects.transcript.responseRetry', {defaultValue: 'Response was not accepted. Your answer is kept; try again.'})}</Alert>}
            {submitted && <Typography role="status" variant="caption">{t('projects.transcript.responseSubmitted', {defaultValue: 'Response sent — waiting for the agent…'})}</Typography>}
            <Stack direction="row" spacing={1}>
                <Button size="small" variant="contained" startIcon={<Check/>} disabled={busy || submitted || (isAsk && !answer.trim())} onClick={() => respond(true)}>
                    {isAsk ? t('projects.transcript.answer', {defaultValue: 'Answer'}) : t('projects.transcript.approve', {defaultValue: 'Allow'})}
                </Button>
                {!isAsk && rememberLabel && (
                    <Button size="small" variant="outlined" disabled={busy || submitted} onClick={() => respond(true, true)}>
                        {rememberLabel(message.content)}
                    </Button>
                )}
                <Button size="small" variant="outlined" color="inherit" startIcon={<Close/>} disabled={busy || submitted} onClick={() => respond(false)}>
                    {isAsk ? t('projects.transcript.decline', {defaultValue: 'Decline'}) : t('projects.transcript.deny', {defaultValue: 'Deny'})}
                </Button>
            </Stack>
        </Paper>
    );
};

interface BlockListProps {
    blocks: TranscriptBlock[];
    pendingRequestId?: string;
    working: boolean;
    expandAll: boolean;
    onRespond: Respond;
    // Marks each top-level message with its block index, so the trajectory
    // can lead back to it (calls and requests carry their own ids).
    anchored?: boolean;
}

// shownBlocks is what a list renders: a settled approval that its call's row
// already reports is left out, and the calls on either side of it read as
// one run again. Each block keeps its index (the trajectory's anchor) and
// whether it ends the list (the live one).
const shownBlocks = (blocks: TranscriptBlock[], pendingRequestId?: string): {block: TranscriptBlock; index: number; last: boolean}[] => {
    const out: {block: TranscriptBlock; index: number; last: boolean}[] = [];
    blocks.forEach((block, index) => {
        const last = index === blocks.length - 1;
        if (block.type === 'request' && block.toolId && block.message.request_id !== pendingRequestId) {
            if (last && out.length > 0) out[out.length - 1].last = true;
            return;
        }
        const previous = out[out.length - 1];
        if (block.type === 'activity' && previous?.block.type === 'activity') {
            out[out.length - 1] = {block: {type: 'activity', steps: [...previous.block.steps, ...block.steps]}, index: previous.index, last};
            return;
        }
        out.push({block, index, last});
    });
    return out;
};

// BlockList renders a run of blocks: the conversation, or a subagent's own
// work inside its card.
const BlockList = ({blocks, pendingRequestId, working, expandAll, onRespond, anchored = false}: BlockListProps) => (
    <>
        {shownBlocks(blocks, pendingRequestId).map(({block: b, index: i, last}) => {
            const anchor = anchored ? {'data-block': i} : {};
            switch (b.type) {
                case 'user':
                    return <Box key={i} {...anchor}><UserBubble message={b.message}/></Box>;
                case 'assistant':
                    return <Box key={i} {...anchor}><AssistantText message={b.message}/></Box>;
                case 'activity':
                    return <ActivityRun key={i} steps={b.steps} live={working && last} expandAll={expandAll} pendingRequestId={pendingRequestId}/>;
                case 'agent':
                    return <AgentCard key={i} block={b} turnLive={working} expandAll={expandAll} pendingRequestId={pendingRequestId} onRespond={onRespond}/>;
                case 'request':
                    return b.message.request_id && b.message.request_id === pendingRequestId
                        ? (
                            <RequestCard
                                key={b.message.request_id}
                                block={b}
                                onRespond={(...args) => onRespond(b.message.request_id ?? '', ...args)}
                            />
                        )
                        : <SettledRequest key={b.message.request_id ?? i} block={b}/>;
                case 'error':
                    return (
                        <Stack key={i} {...anchor} direction="row" spacing={0.75} sx={{alignItems: 'flex-start', color: (theme) => getReadableAccent(theme, 'error')}}>
                            <ErrorOutline sx={{fontSize: 18, mt: 0.25}}/>
                            <Typography variant="body2" sx={{color: 'inherit', whiteSpace: 'pre-wrap', wordBreak: 'break-word'}}>{b.message.content}</Typography>
                        </Stack>
                    );
                case 'system':
                    return (
                        <Typography key={i} {...anchor} variant="body2" sx={{color: 'text.secondary', textAlign: 'center'}}>
                            {b.message.content}
                        </Typography>
                    );
            }
        })}
    </>
);

// AgentCard is one subagent run: what it was asked, how it is going (its
// current action, tools, tokens, time), and — expanded — everything it did
// and the report it handed back. Its own work stays inside the card so the
// conversation reads as the main agent's.
const AgentCard = ({block, turnLive, expandAll, pendingRequestId, onRespond}: {
    block: Extract<TranscriptBlock, {type: 'agent'}>;
    turnLive: boolean;
    expandAll: boolean;
    pendingRequestId?: string;
    onRespond: Respond;
}) => {
    const {t} = useTranslation();
    const [open, toggle] = useExpanded(expandAll);
    const input = (block.call.input ?? {}) as {description?: string; prompt?: string; subagent_type?: string};
    const task = block.task;
    const status = agentStatus(block, turnLive);
    const running = status === 'running';
    const report = agentReport(block);
    const usage = task?.usage;
    const meta = usage
        ? [
            t('projects.transcript.toolCount', {defaultValue: usage.tool_uses === 1 ? '1 tool' : '{{count}} tools', count: usage.tool_uses}),
            `${formatTokens(usage.total_tokens)} ${t('projects.transcript.tokens', {defaultValue: 'tokens'})}`,
            formatDuration(usage.duration_ms),
        ].join(' · ')
        : '';

    const statusIcon = {
        running: <CircularProgress size={14}/>,
        completed: <CheckCircle sx={{fontSize: 16, color: (theme) => getReadableAccent(theme, 'success')}}/>,
        stopped: <Block sx={{fontSize: 16, color: 'text.secondary'}}/>,
        failed: <Cancel sx={{fontSize: 16, color: (theme) => getReadableAccent(theme, 'error')}}/>,
    }[status];

    return (
        <Paper variant="outlined" data-call-ids={block.call.id} sx={{borderRadius: 1.5, overflow: 'hidden'}}>
            <Box
                role="button"
                aria-expanded={open}
                onClick={toggle}
                sx={{px: 1.5, py: 1, cursor: 'default', userSelect: 'none', '&:hover': {bgcolor: 'action.hover'}}}
            >
                <Stack direction="row" spacing={1} sx={{alignItems: 'center', minWidth: 0}}>
                    <Robot sx={{fontSize: 18, color: 'text.secondary', flexShrink: 0}}/>
                    <Typography variant="body2" noWrap sx={{fontWeight: 600, color: 'text.primary', minWidth: 0}}>
                        {input.description || t('projects.transcript.subagent', {defaultValue: 'Subagent'})}
                    </Typography>
                    {(task?.subagentType || input.subagent_type) && (
                        <Typography variant="caption" sx={{color: 'text.secondary', flexShrink: 0}}>
                            {task?.subagentType || input.subagent_type}
                        </Typography>
                    )}
                    {task?.background && (
                        <Chip size="small" variant="outlined" label={t('projects.transcript.background', {defaultValue: 'background'})} sx={{height: 18, '& .MuiChip-label': {px: 0.75, fontSize: fontSizes.xs}}}/>
                    )}
                    <Box sx={{flex: 1}}/>
                    <Stack direction="row" spacing={0.5} sx={{alignItems: 'center', flexShrink: 0}} title={statusLabel(t, status)}>
                        {statusIcon}
                    </Stack>
                    {open ? <ExpandMore sx={{fontSize: 16, color: 'text.secondary'}}/> : <ChevronRight sx={{fontSize: 16, color: 'text.secondary'}}/>}
                </Stack>
                {(running && task?.activity) || meta ? (
                    <Typography variant="caption" noWrap component="div" sx={{color: 'text.secondary', pl: 3.25, mt: 0.25}}>
                        {[running ? task?.activity : statusLabel(t, status), meta].filter(Boolean).join(' · ')}
                    </Typography>
                ) : null}
            </Box>
            <Collapse in={open} unmountOnExit>
                <Stack spacing={1.5} sx={{px: 1.5, pb: 1.5, pt: 0.5, borderTop: 1, borderColor: 'divider'}}>
                    {input.prompt && (
                        <Box>
                            <Typography variant="caption" sx={{color: 'text.secondary'}}>{t('projects.transcript.agentPrompt', {defaultValue: 'Asked to'})}</Typography>
                            <Typography variant="body2" sx={{color: 'text.primary', whiteSpace: 'pre-wrap', wordBreak: 'break-word', maxHeight: 160, overflow: 'auto'}}>
                                {input.prompt}
                            </Typography>
                        </Box>
                    )}
                    {/* The report is its last reply, shown below; the run
                        above it is everything before. */}
                    <BlockList
                        blocks={report && block.children[block.children.length - 1]?.type === 'assistant' ? block.children.slice(0, -1) : block.children}
                        pendingRequestId={pendingRequestId}
                        working={running}
                        expandAll={expandAll}
                        onRespond={onRespond}
                    />
                    {report && (
                        <Box>
                            <Typography variant="caption" sx={{color: 'text.secondary'}}>{t('projects.transcript.agentReport', {defaultValue: 'Report'})}</Typography>
                            <Markdown content={report}/>
                        </Box>
                    )}
                    {running && block.children.length === 0 && (
                        <Typography variant="body2" sx={{color: 'text.secondary'}}>{t('projects.transcript.working', {defaultValue: 'Working…'})}</Typography>
                    )}
                </Stack>
            </Collapse>
        </Paper>
    );
};

const Transcript = ({blocks, pendingRequestId, working, expandAll = false, onRespond, drafts, codeBlock, rememberLabel}: TranscriptProps) => {
    const {t} = useTranslation();
    const last = shownBlocks(blocks, pendingRequestId).at(-1)?.block;
    // The spinner rides on the last activity line while a turn runs; if the
    // turn hasn't produced any activity yet, it gets a line of its own.
    const showWorking = working && !pendingRequestId && last?.type !== 'activity';

    return (
        <TranscriptHostProvider drafts={drafts} codeBlock={codeBlock} rememberLabel={rememberLabel}>
            <Stack spacing={2.5}>
                <BlockList anchored blocks={blocks} pendingRequestId={pendingRequestId} working={working} expandAll={expandAll} onRespond={onRespond}/>
                {showWorking && (
                    <Stack direction="row" spacing={1} sx={{alignItems: 'center', color: 'text.secondary'}}>
                        <CircularProgress size={12}/>
                        <Typography variant="body2">{t('projects.transcript.working', {defaultValue: 'Working…'})}</Typography>
                    </Stack>
                )}
            </Stack>
        </TranscriptHostProvider>
    );
};

export default Transcript;
