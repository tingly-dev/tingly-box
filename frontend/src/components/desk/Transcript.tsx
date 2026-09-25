import {Block, Cancel, Check, CheckCircle, ChevronRight, Close, ErrorOutline, ExpandMore, Robot} from '@/components/icons';
import type {MessageInfo} from '@/services/deskApi';
import {Box, Button, Chip, CircularProgress, Collapse, Paper, Stack, TextField, Typography} from '@mui/material';
import type {TFunction} from 'i18next';
import {useState} from 'react';
import {useTranslation} from 'react-i18next';
import type {ActivityStep, TaskState, TranscriptBlock} from './deskUtils';
import {agentReport, formatTokens, toolSummary} from './deskUtils';
import Markdown from './Markdown';

interface TranscriptProps {
    blocks: TranscriptBlock[];
    pendingRequestId?: string;
    working: boolean;
    // Opens every activity row; each row can still be toggled on its own.
    expandAll?: boolean;
    onRespond: (requestId: string, approved: boolean, answer: string) => Promise<void>;
}

const mono = {fontFamily: 'monospace', fontSize: '0.8rem'};

// The theme's body variants default to the secondary text color, which suits
// metadata. The conversation itself is the subject of this page, so it uses
// the primary color at reading size; labels, notes and tool detail stay
// secondary so they recede.
const conversationText = {
    color: 'text.primary', fontSize: '0.9375rem', lineHeight: 1.7, whiteSpace: 'pre-wrap', wordBreak: 'break-word',
} as const;

const UserBubble = ({message}: {message: MessageInfo}) => (
    <Box sx={{display: 'flex', justifyContent: 'flex-end'}}>
        <Box sx={{maxWidth: '80%', bgcolor: 'action.hover', px: 2, py: 1.25, borderRadius: 3}}>
            <Typography variant="body1" sx={conversationText}>{message.content}</Typography>
        </Box>
    </Box>
);

const AssistantText = ({message}: {message: MessageInfo}) => <Markdown content={message.content}/>;

const formatDuration = (ms: number): string => {
    const sec = Math.round(ms / 1000);
    return sec < 60 ? `${sec}s` : `${Math.floor(sec / 60)}m ${sec % 60}s`;
};

const statusLabel = (t: TFunction, status: TaskState['status']) => ({
    running: t('desk.taskRunning', {defaultValue: 'running'}),
    completed: t('desk.taskDone', {defaultValue: 'done'}),
    stopped: t('desk.taskStopped', {defaultValue: 'stopped'}),
    failed: t('desk.taskFailed', {defaultValue: 'failed'}),
}[status]);

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
            label={`${t('desk.background', {defaultValue: 'background'})} · ${statusLabel(t, task.status)}`}
            sx={{height: 20, flexShrink: 0, '& .MuiChip-label': {px: 0.75, fontSize: '0.7rem'}}}
        />
    );
};

const StepDetail = ({step}: {step: ActivityStep}) => {
    if (step.type === 'thinking') {
        return (
            <Typography variant="body2" color="text.secondary" sx={{fontStyle: 'italic', whiteSpace: 'pre-wrap'}}>
                {step.text}
            </Typography>
        );
    }
    const summary = toolSummary(step.input);
    return (
        <Box>
            <Stack direction="row" spacing={1} sx={{alignItems: 'baseline', minWidth: 0}}>
                <Typography variant="body2" sx={{fontWeight: 600, flexShrink: 0, color: step.isError ? 'error.main' : 'text.primary'}}>
                    {step.name || 'Result'}
                </Typography>
                {summary && (
                    <Typography variant="body2" color="text.secondary" noWrap sx={mono}>{summary}</Typography>
                )}
                {step.task?.background && <TaskTag task={step.task}/>}
            </Stack>
            {step.result && (
                <Box
                    component="pre"
                    sx={{
                        ...mono, m: 0, mt: 0.5, p: 1, borderRadius: 1, maxHeight: 220, overflow: 'auto',
                        whiteSpace: 'pre-wrap', wordBreak: 'break-word',
                        bgcolor: 'action.hover',
                        color: 'text.secondary',
                    }}
                >
                    {step.result}
                </Box>
            )}
        </Box>
    );
};

// ActivityRow is one line for a run of thinking and tool calls ("Used 3
// tools"), expandable into each call and its output: the reply stays the
// visual anchor, and the detail is one click away rather than in the way.
const ActivityRow = ({steps, live, expandAll}: {steps: ActivityStep[]; live: boolean; expandAll: boolean}) => {
    const {t} = useTranslation();
    // A row's own toggle holds until "expand all" is flipped again.
    const [toggled, setToggled] = useState<{under: boolean; open: boolean} | null>(null);
    const open = toggled?.under === expandAll ? toggled.open : expandAll;
    const setOpen = (next: (v: boolean) => boolean) => setToggled({under: expandAll, open: next(open)});
    const tools = steps.filter((s) => s.type === 'tool');
    const failed = tools.filter((s) => s.type === 'tool' && s.isError).length;

    const label = tools.length === 0
        ? t('desk.thought', {defaultValue: 'Thought'})
        : t('desk.usedTools', {defaultValue: tools.length === 1 ? 'Used 1 tool' : 'Used {{count}} tools', count: tools.length});

    return (
        <Box>
            <Box
                role="button"
                onClick={() => setOpen((v) => !v)}
                sx={{
                    display: 'inline-flex', alignItems: 'center', gap: 0.5, cursor: 'pointer', userSelect: 'none',
                    color: 'text.secondary', '&:hover': {color: 'text.primary'},
                }}
            >
                {live && <CircularProgress size={12} sx={{mr: 0.5}}/>}
                <Typography variant="body2" sx={{color: 'inherit'}}>{label}</Typography>
                {/* A failing command is routine inside an agent's run (a red
                    test it goes on to fix), so only the count is marked. */}
                {failed > 0 && (
                    <Typography variant="body2" sx={{color: 'error.main'}}>
                        · {t('desk.failedCount', {defaultValue: '{{count}} failed', count: failed})}
                    </Typography>
                )}
                {open ? <ExpandMore sx={{fontSize: 16}}/> : <ChevronRight sx={{fontSize: 16}}/>}
            </Box>
            <Collapse in={open} unmountOnExit>
                <Stack spacing={1.25} sx={{mt: 1, pl: 1.5, borderLeft: 2, borderColor: 'divider'}}>
                    {steps.map((s, i) => <StepDetail key={i} step={s}/>)}
                </Stack>
            </Collapse>
        </Box>
    );
};

// RequestCard is an approval or a question from Claude. Only the one still
// waiting on a live turn is actionable; an answered one collapses to a line
// saying what was decided.
const RequestCard = ({block, pending, onRespond}: {
    block: Extract<TranscriptBlock, {type: 'request'}>;
    pending: boolean;
    onRespond: (approved: boolean, answer: string) => Promise<void>;
}) => {
    const {t} = useTranslation();
    const [answer, setAnswer] = useState('');
    const [busy, setBusy] = useState(false);
    const {message, response} = block;
    const isAsk = message.kind === 'ask_request';
    const summary = isAsk ? '' : toolSummary(message.payload);

    const respond = async (approved: boolean) => {
        setBusy(true);
        try {
            await onRespond(approved, answer);
        } finally {
            setBusy(false);
        }
    };

    if (!pending) {
        const outcome = response
            ? (isAsk ? response.content || t('desk.noAnswer', {defaultValue: '(no answer)'}) : response.content)
            : t('desk.unanswered', {defaultValue: 'not answered'});
        return (
            <Typography variant="body2" color="text.secondary" noWrap>
                {isAsk ? message.content : `${message.content}${summary ? ` · ${summary}` : ''}`} → {outcome}
            </Typography>
        );
    }

    return (
        <Paper variant="outlined" sx={{p: 1.5, borderRadius: 2, borderColor: 'primary.main'}}>
            <Typography variant="body2" sx={{fontWeight: 600, mb: 0.75, color: 'text.primary', fontSize: '0.875rem'}}>
                {isAsk
                    ? message.content
                    : t('desk.approvalTitle', {defaultValue: 'Allow {{tool}}?', tool: message.content})}
            </Typography>
            {summary && (
                <Box component="pre" sx={{...mono, m: 0, mb: 1, p: 1, borderRadius: 1, bgcolor: 'action.hover', whiteSpace: 'pre-wrap', wordBreak: 'break-word'}}>
                    {summary}
                </Box>
            )}
            {isAsk && (
                <TextField
                    size="small"
                    fullWidth
                    autoFocus
                    placeholder={t('desk.answerPlaceholder', {defaultValue: 'Your answer'})}
                    value={answer}
                    onChange={(e) => setAnswer(e.target.value)}
                    sx={{mb: 1}}
                />
            )}
            <Stack direction="row" spacing={1}>
                <Button size="small" variant="contained" startIcon={<Check/>} disabled={busy} onClick={() => respond(true)}>
                    {isAsk ? t('desk.answer', {defaultValue: 'Answer'}) : t('desk.approve', {defaultValue: 'Allow'})}
                </Button>
                <Button size="small" variant="outlined" color="inherit" startIcon={<Close/>} disabled={busy} onClick={() => respond(false)}>
                    {isAsk ? t('desk.decline', {defaultValue: 'Decline'}) : t('desk.deny', {defaultValue: 'Deny'})}
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
    onRespond: (requestId: string, approved: boolean, answer: string) => Promise<void>;
}

// BlockList renders a run of blocks: the conversation, or a subagent's own
// work inside its card.
const BlockList = ({blocks, pendingRequestId, working, expandAll, onRespond}: BlockListProps) => (
    <>
        {blocks.map((b, i) => {
            switch (b.type) {
                case 'user':
                    return <UserBubble key={i} message={b.message}/>;
                case 'assistant':
                    return <AssistantText key={i} message={b.message}/>;
                case 'activity':
                    return <ActivityRow key={i} steps={b.steps} live={working && i === blocks.length - 1} expandAll={expandAll}/>;
                case 'agent':
                    return <AgentCard key={i} block={b} turnLive={working} expandAll={expandAll} onRespond={onRespond}/>;
                case 'request':
                    return (
                        <RequestCard
                            key={i}
                            block={b}
                            pending={b.message.request_id === pendingRequestId}
                            onRespond={(approved, answer) => onRespond(b.message.request_id ?? '', approved, answer)}
                        />
                    );
                case 'error':
                    return (
                        <Stack key={i} direction="row" spacing={0.75} sx={{alignItems: 'flex-start', color: 'error.main'}}>
                            <ErrorOutline sx={{fontSize: 18, mt: 0.25}}/>
                            <Typography variant="body2" sx={{color: 'inherit', whiteSpace: 'pre-wrap', wordBreak: 'break-word'}}>{b.message.content}</Typography>
                        </Stack>
                    );
                case 'system':
                    return (
                        <Typography key={i} variant="body2" sx={{color: 'text.secondary', textAlign: 'center'}}>
                            {b.message.content}
                        </Typography>
                    );
            }
        })}
    </>
);

// agentStatus is the card's state. Without task events (older Claude Code)
// the call's result means it finished; a foreground run left "running"
// after its turn ended was cut off with the turn.
const agentStatus = (block: Extract<TranscriptBlock, {type: 'agent'}>, turnLive: boolean): TaskState['status'] => {
    const status = block.task?.status ?? (block.call.result !== undefined ? 'completed' : 'running');
    if (status === 'running' && !turnLive && !block.task?.background) return 'stopped';
    return status;
};

// AgentCard is one subagent run: what it was asked, how it is going (its
// current action, tools, tokens, time), and — expanded — everything it did
// and the report it handed back. Its own work stays inside the card so the
// conversation reads as the main agent's.
const AgentCard = ({block, turnLive, expandAll, onRespond}: {
    block: Extract<TranscriptBlock, {type: 'agent'}>;
    turnLive: boolean;
    expandAll: boolean;
    onRespond: (requestId: string, approved: boolean, answer: string) => Promise<void>;
}) => {
    const {t} = useTranslation();
    const [toggled, setToggled] = useState<{under: boolean; open: boolean} | null>(null);
    const open = toggled?.under === expandAll ? toggled.open : expandAll;
    const input = (block.call.input ?? {}) as {description?: string; prompt?: string; subagent_type?: string};
    const task = block.task;
    const status = agentStatus(block, turnLive);
    const running = status === 'running';
    const report = agentReport(block);
    const usage = task?.usage;
    const meta = usage
        ? [
            t('desk.toolCount', {defaultValue: usage.tool_uses === 1 ? '1 tool' : '{{count}} tools', count: usage.tool_uses}),
            `${formatTokens(usage.total_tokens)} ${t('desk.tokens', {defaultValue: 'tokens'})}`,
            formatDuration(usage.duration_ms),
        ].join(' · ')
        : '';

    const statusIcon = {
        running: <CircularProgress size={14}/>,
        completed: <CheckCircle sx={{fontSize: 16, color: 'success.main'}}/>,
        stopped: <Block sx={{fontSize: 16, color: 'text.secondary'}}/>,
        failed: <Cancel sx={{fontSize: 16, color: 'error.main'}}/>,
    }[status];

    return (
        <Paper variant="outlined" sx={{borderRadius: 2, overflow: 'hidden'}}>
            <Box
                role="button"
                aria-expanded={open}
                onClick={() => setToggled({under: expandAll, open: !open})}
                sx={{px: 1.5, py: 1, cursor: 'pointer', userSelect: 'none', '&:hover': {bgcolor: 'action.hover'}}}
            >
                <Stack direction="row" spacing={1} sx={{alignItems: 'center', minWidth: 0}}>
                    <Robot sx={{fontSize: 18, color: 'text.secondary', flexShrink: 0}}/>
                    <Typography variant="body2" noWrap sx={{fontWeight: 600, color: 'text.primary', minWidth: 0}}>
                        {input.description || t('desk.subagent', {defaultValue: 'Subagent'})}
                    </Typography>
                    {(task?.subagentType || input.subagent_type) && (
                        <Typography variant="caption" sx={{color: 'text.secondary', flexShrink: 0}}>
                            {task?.subagentType || input.subagent_type}
                        </Typography>
                    )}
                    {task?.background && (
                        <Chip size="small" variant="outlined" label={t('desk.background', {defaultValue: 'background'})} sx={{height: 18, '& .MuiChip-label': {px: 0.75, fontSize: '0.7rem'}}}/>
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
                            <Typography variant="caption" sx={{color: 'text.secondary'}}>{t('desk.agentPrompt', {defaultValue: 'Asked to'})}</Typography>
                            <Typography variant="body2" sx={{color: 'text.primary', whiteSpace: 'pre-wrap', wordBreak: 'break-word', maxHeight: 160, overflow: 'auto'}}>
                                {input.prompt}
                            </Typography>
                        </Box>
                    )}
                    {/* The report is its last reply, shown below; the run
                        above it is everything before. */}
                    <BlockList
                        blocks={report && block.children[block.children.length - 1]?.type === 'assistant' ? block.children.slice(0, -1) : block.children}
                        working={running}
                        expandAll={expandAll}
                        onRespond={onRespond}
                    />
                    {report && (
                        <Box>
                            <Typography variant="caption" sx={{color: 'text.secondary'}}>{t('desk.agentReport', {defaultValue: 'Report'})}</Typography>
                            <Markdown content={report}/>
                        </Box>
                    )}
                    {running && block.children.length === 0 && (
                        <Typography variant="body2" sx={{color: 'text.secondary'}}>{t('desk.working', {defaultValue: 'Working…'})}</Typography>
                    )}
                </Stack>
            </Collapse>
        </Paper>
    );
};

const Transcript = ({blocks, pendingRequestId, working, expandAll = false, onRespond}: TranscriptProps) => {
    const {t} = useTranslation();
    const last = blocks[blocks.length - 1];
    // The spinner rides on the last activity row while a turn runs; if the
    // turn hasn't produced any activity yet, it gets a line of its own.
    const showWorking = working && !pendingRequestId && last?.type !== 'activity';

    return (
        <Stack spacing={2.5}>
            <BlockList blocks={blocks} pendingRequestId={pendingRequestId} working={working} expandAll={expandAll} onRespond={onRespond}/>
            {showWorking && (
                <Stack direction="row" spacing={1} sx={{alignItems: 'center', color: 'text.secondary'}}>
                    <CircularProgress size={12}/>
                    <Typography variant="body2">{t('desk.working', {defaultValue: 'Working…'})}</Typography>
                </Stack>
            )}
        </Stack>
    );
};

export default Transcript;
