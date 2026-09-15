// The conversation view of a task's event log, in the shape people know
// from Claude Code and Codex: the user's turns on the right, the agent's
// text plain on the left, and what the agent *did* in between as compact
// activity rows — each tool call on one line, its output one click away.
// A permission question is a card in the flow with the answer controls on
// it; once answered it stays as a record. Status noise (running/idle) is
// not in the transcript at all — the page shows that live.
import {Fragment, useMemo, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {Box, Button, Chip, Collapse, Paper, Stack, TextField, Typography} from '@mui/material';
import {alpha} from '@mui/material/styles';
import CodeBlock from '@/components/CodeBlock';
import {
    AutoAwesome as IconAgent, Build as IconTool, ChevronRight, Code as IconCode, Description as IconFile,
    ExpandMore, Psychology as IconThinking, Search as IconSearch, Terminal as IconTerminal, Warning as IconWarning,
} from '@/components/icons';
import type {AgentEvent} from '@/services/agentApi';

export interface PendingRequest {
    requestId: string;
    kind: 'approval' | 'ask';
}

interface Props {
    events: AgentEvent[];
    // Requests still awaiting an answer (the session is waiting_input).
    pending: PendingRequest[];
    onRespond: (requestId: string, approved: boolean, answer?: string) => Promise<void>;
}

const payloadOf = (e: AgentEvent): Record<string, unknown> => {
    if (!e.payload) return {};
    if (typeof e.payload === 'string') {
        try {
            return JSON.parse(e.payload);
        } catch {
            return {};
        }
    }
    return e.payload as Record<string, unknown>;
};

// One line that says what a tool call is about: the command, the file, the
// pattern — whatever the tool's input names first.
const briefInput = (input: unknown): string => {
    if (!input || typeof input !== 'object') return '';
    const m = input as Record<string, unknown>;
    const pick = m.command ?? m.file_path ?? m.path ?? m.pattern ?? m.url ?? m.query ?? m.description;
    const s = typeof pick === 'string' ? pick : JSON.stringify(m);
    return s.length > 160 ? `${s.slice(0, 157)}…` : s;
};

const toolIcon = (name: string) => {
    const n = name.toLowerCase();
    if (n === 'bash') return <IconTerminal fontSize="inherit" />;
    if (n === 'read' || n === 'write' || n === 'notebookedit') return <IconFile fontSize="inherit" />;
    if (n === 'edit' || n === 'multiedit') return <IconCode fontSize="inherit" />;
    if (n === 'grep' || n === 'glob' || n === 'websearch' || n === 'webfetch') return <IconSearch fontSize="inherit" />;
    return <IconTool fontSize="inherit" />;
};

// Assistant text with fenced code blocks rendered as code; everything else
// as paragraphs. Not a markdown engine — enough that command output and
// snippets do not collapse into a wall of text.
const RichText = ({text}: {text: string}) => {
    const parts = useMemo(() => {
        const out: Array<{code: string; lang?: string} | {text: string}> = [];
        const re = /```([\w+-]*)\n([\s\S]*?)```/g;
        let last = 0;
        let m: RegExpExecArray | null;
        while ((m = re.exec(text)) !== null) {
            if (m.index > last) out.push({text: text.slice(last, m.index)});
            out.push({code: m[2].replace(/\n$/, ''), lang: m[1] || undefined});
            last = m.index + m[0].length;
        }
        if (last < text.length) out.push({text: text.slice(last)});
        return out;
    }, [text]);
    return (
        <Stack spacing={1}>
            {parts.map((p, i) =>
                'code' in p ? (
                    <CodeBlock key={i} code={p.code} language={p.lang} showCopy />
                ) : (
                    <Typography key={i} variant="body2" sx={{whiteSpace: 'pre-wrap', wordBreak: 'break-word', lineHeight: 1.65}}>
                        {p.text.trim()}
                    </Typography>
                ),
            )}
        </Stack>
    );
};

const UserTurn = ({text}: {text: string}) => (
    <Box sx={{display: 'flex', justifyContent: 'flex-end', pl: {xs: 4, sm: 10}}}>
        <Box
            sx={(theme) => ({
                px: 1.75, py: 1.1, borderRadius: 2.5, borderTopRightRadius: 6,
                bgcolor: alpha(theme.palette.primary.main, theme.palette.mode === 'dark' ? 0.22 : 0.1),
                whiteSpace: 'pre-wrap', wordBreak: 'break-word',
            })}
        >
            <Typography variant="body2" sx={{lineHeight: 1.6}}>{text}</Typography>
        </Box>
    </Box>
);

const AgentTurn = ({children}: {children: React.ReactNode}) => (
    <Stack direction="row" spacing={1.5} sx={{pr: {xs: 0, sm: 6}}}>
        <Box sx={{mt: 0.4, color: 'primary.main', fontSize: 18, flexShrink: 0, display: 'flex'}}><IconAgent fontSize="inherit" /></Box>
        <Box sx={{minWidth: 0, flex: 1}}>{children}</Box>
    </Stack>
);

interface ToolCall {
    use: AgentEvent;
    result?: AgentEvent;
    // A thinking block rides in the activity block as a foldable row too.
    thinking?: boolean;
}

const ThinkingRow = ({event}: {event: AgentEvent}) => {
    const {t} = useTranslation();
    const [open, setOpen] = useState(false);
    const text = event.text ?? '';
    const preview = text.replace(/\s+/g, ' ').trim();
    return (
        <Box>
            <Stack
                direction="row"
                spacing={1}
                onClick={() => setOpen((v) => !v)}
                sx={{alignItems: 'center', py: 0.5, px: 1, borderRadius: 1, minWidth: 0, cursor: 'pointer', '&:hover': {bgcolor: 'action.hover'}}}
            >
                <Box sx={{fontSize: 15, display: 'flex', color: 'text.secondary', flexShrink: 0}}><IconThinking fontSize="inherit" /></Box>
                <Typography variant="caption" sx={{fontWeight: 600, flexShrink: 0, color: 'text.secondary'}}>{t('tasks.detail.thinking')}</Typography>
                {!open && (
                    <Typography variant="caption" color="text.disabled" sx={{flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontStyle: 'italic'}}>
                        {preview}
                    </Typography>
                )}
                {open && <Box sx={{flex: 1}} />}
                <Box sx={{display: 'flex', color: 'text.disabled', fontSize: 16, flexShrink: 0}}>
                    {open ? <ExpandMore fontSize="inherit" /> : <ChevronRight fontSize="inherit" />}
                </Box>
            </Stack>
            <Collapse in={open} unmountOnExit>
                <Typography variant="body2" color="text.secondary" sx={{pl: 4, pr: 1, pb: 1, whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontStyle: 'italic'}}>
                    {text}
                </Typography>
            </Collapse>
        </Box>
    );
};

const ToolRow = ({call}: {call: ToolCall}) => {
    const {t} = useTranslation();
    const [open, setOpen] = useState(false);
    const input = payloadOf(call.use);
    const resultPayload = call.result ? payloadOf(call.result) : {};
    const isError = resultPayload.is_error === true;
    const output = call.result?.text ?? '';
    const hasOutput = output.trim().length > 0;
    const name = call.use.text || 'tool';
    return (
        <Box>
            <Stack
                direction="row"
                spacing={1}
                onClick={hasOutput ? () => setOpen((v) => !v) : undefined}
                sx={{
                    alignItems: 'center', py: 0.5, px: 1, borderRadius: 1, minWidth: 0,
                    cursor: hasOutput ? 'pointer' : 'default',
                    '&:hover': hasOutput ? {bgcolor: 'action.hover'} : undefined,
                }}
            >
                <Box sx={{fontSize: 15, display: 'flex', color: isError ? 'error.main' : 'text.secondary', flexShrink: 0}}>
                    {isError ? <IconWarning fontSize="inherit" /> : toolIcon(name)}
                </Box>
                <Typography variant="caption" sx={{fontWeight: 600, flexShrink: 0, color: isError ? 'error.main' : 'text.primary'}}>{name}</Typography>
                <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{fontFamily: 'monospace', flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap'}}
                    title={briefInput(input)}
                >
                    {briefInput(input)}
                </Typography>
                {!call.result && <Typography variant="caption" color="text.disabled">{t('tasks.detail.toolPending')}</Typography>}
                {hasOutput && (
                    <Box sx={{display: 'flex', color: 'text.disabled', fontSize: 16, flexShrink: 0}}>
                        {open ? <ExpandMore fontSize="inherit" /> : <ChevronRight fontSize="inherit" />}
                    </Box>
                )}
            </Stack>
            {hasOutput && (
                <Collapse in={open} unmountOnExit>
                    <Box sx={{pl: 4, pr: 1, pb: 1}}>
                        <Typography
                            component="pre"
                            variant="caption"
                            sx={{
                                m: 0, p: 1.25, borderRadius: 1, bgcolor: 'action.hover', fontFamily: 'monospace',
                                whiteSpace: 'pre-wrap', wordBreak: 'break-word', maxHeight: 320, overflow: 'auto',
                                color: isError ? 'error.main' : 'text.primary',
                            }}
                        >
                            {output}
                        </Typography>
                    </Box>
                </Collapse>
            )}
        </Box>
    );
};

const ActivityGroup = ({calls}: {calls: ToolCall[]}) => (
    <Box sx={{pl: {xs: 0, sm: 4.5}, pr: {xs: 0, sm: 6}}}>
        <Paper variant="outlined" sx={{py: 0.5, px: 0.5, borderRadius: 2, bgcolor: 'background.default'}}>
            {calls.map((c) => (c.thinking ? <ThinkingRow key={c.use.seq} event={c.use} /> : <ToolRow key={c.use.seq} call={c} />))}
        </Paper>
    </Box>
);

const RequestCard = ({event, pending, resolution, onRespond}: {
    event: AgentEvent;
    pending?: PendingRequest;
    resolution?: AgentEvent;
    onRespond: Props['onRespond'];
}) => {
    const {t} = useTranslation();
    const [answer, setAnswer] = useState('');
    const [busy, setBusy] = useState(false);
    const payload = payloadOf(event);
    const isAsk = event.kind === 'ask_request';
    const title = isAsk
        ? t('tasks.detail.askTitle')
        : t('tasks.detail.approvalTitle', {tool: event.text || (payload.tool as string) || 'tool'});
    const detail = isAsk ? (event.text || (payload.message as string) || '') : briefInput(payload.input);
    const outcome = resolution?.text ?? '';
    const approved = /^approved|^allow/i.test(outcome);

    const act = async (yes: boolean) => {
        setBusy(true);
        try {
            await onRespond(event.request_id ?? '', yes, isAsk ? answer : undefined);
        } finally {
            setBusy(false);
        }
    };

    return (
        <Box sx={{pl: {xs: 0, sm: 4.5}, pr: {xs: 0, sm: 6}}}>
            <Paper
                variant="outlined"
                sx={(theme) => ({
                    p: 1.75, borderRadius: 2,
                    borderColor: pending ? theme.palette.warning.main : 'divider',
                    borderLeftWidth: 3,
                    borderLeftColor: pending ? theme.palette.warning.main : approved ? theme.palette.success.main : resolution ? theme.palette.text.disabled : 'divider',
                    bgcolor: pending ? alpha(theme.palette.warning.main, 0.06) : 'background.paper',
                })}
            >
                <Stack spacing={1.25}>
                    <Stack direction="row" spacing={1} sx={{alignItems: 'center'}}>
                        <Typography variant="subtitle2" sx={{flex: 1}}>{title}</Typography>
                        {resolution && (
                            <Chip
                                size="small"
                                variant="outlined"
                                color={isAsk ? 'default' : approved ? 'success' : 'default'}
                                label={isAsk ? t('tasks.detail.answered') : approved ? t('tasks.detail.allowed') : t('tasks.detail.denied')}
                            />
                        )}
                    </Stack>
                    {detail && (isAsk ? (
                        <Typography variant="body2" sx={{whiteSpace: 'pre-wrap', wordBreak: 'break-word'}}>{detail}</Typography>
                    ) : (
                        <Typography component="pre" variant="body2" sx={{m: 0, p: 1, borderRadius: 1, bgcolor: 'action.hover', fontFamily: 'monospace', whiteSpace: 'pre-wrap', wordBreak: 'break-word'}}>
                            {detail}
                        </Typography>
                    ))}
                    {pending && isAsk && (
                        <TextField
                            size="small"
                            fullWidth
                            multiline
                            maxRows={4}
                            autoFocus
                            placeholder={t('tasks.detail.answerPlaceholder')}
                            value={answer}
                            onChange={(e) => setAnswer(e.target.value)}
                        />
                    )}
                    {pending && (
                        // Full-width buttons on phones: this is the tap that unblocks the agent.
                        <Stack direction={{xs: 'column', sm: 'row'}} spacing={1}>
                            {isAsk ? (
                                <Button variant="contained" disabled={busy || !answer.trim()} onClick={() => act(true)}>
                                    {t('tasks.detail.answer')}
                                </Button>
                            ) : (
                                <>
                                    <Button variant="contained" disabled={busy} onClick={() => act(true)}>
                                        {t('tasks.detail.approve')}
                                    </Button>
                                    <Button variant="outlined" color="inherit" disabled={busy} onClick={() => act(false)}>
                                        {t('tasks.detail.deny')}
                                    </Button>
                                </>
                            )}
                        </Stack>
                    )}
                </Stack>
            </Paper>
        </Box>
    );
};

type Block =
    | {kind: 'user'; event: AgentEvent}
    | {kind: 'agent'; event: AgentEvent}
    | {kind: 'activity'; calls: ToolCall[]}
    | {kind: 'request'; event: AgentEvent}
    | {kind: 'error'; event: AgentEvent}
    | {kind: 'marker'; event: AgentEvent; note: string}
    | {kind: 'setup'; items: AgentEvent[]};

// Fold the flat log into conversation blocks: tool_use + its tool_result
// become one call; consecutive calls become one activity group; response
// events attach to their request instead of standing alone.
const toBlocks = (events: AgentEvent[]): {blocks: Block[]; responses: Map<string, AgentEvent>} => {
    const blocks: Block[] = [];
    const responses = new Map<string, AgentEvent>();
    const openCalls = new Map<string, ToolCall>();
    const last = () => blocks[blocks.length - 1];
    for (const e of events) {
        switch (e.kind) {
            case 'user_message':
                blocks.push({kind: 'user', event: e});
                break;
            case 'assistant_message':
                blocks.push({kind: 'agent', event: e});
                break;
            case 'thinking': {
                const row: ToolCall = {use: e, thinking: true};
                const l = last();
                if (l && l.kind === 'activity') l.calls.push(row);
                else blocks.push({kind: 'activity', calls: [row]});
                break;
            }
            case 'tool_use': {
                const call: ToolCall = {use: e};
                if (e.request_id) openCalls.set(e.request_id, call);
                const l = last();
                if (l && l.kind === 'activity') l.calls.push(call);
                else blocks.push({kind: 'activity', calls: [call]});
                break;
            }
            case 'tool_result': {
                const call = e.request_id ? openCalls.get(e.request_id) : undefined;
                if (call) {
                    call.result = e;
                    openCalls.delete(e.request_id as string);
                } else {
                    const orphan: ToolCall = {use: {...e, text: 'result'}, result: e};
                    const l = last();
                    if (l && l.kind === 'activity') l.calls.push(orphan);
                    else blocks.push({kind: 'activity', calls: [orphan]});
                }
                break;
            }
            case 'approval_request':
            case 'ask_request':
                blocks.push({kind: 'request', event: e});
                break;
            case 'approval_response':
            case 'ask_response':
                if (e.request_id) responses.set(e.request_id, e);
                break;
            case 'error':
                blocks.push({kind: 'error', event: e});
                break;
            case 'status': {
                // Only the markers a reader needs: an interrupted turn, a failure.
                const text = e.text ?? '';
                if (text.startsWith('idle: interrupted')) blocks.push({kind: 'marker', event: e, note: 'interrupted'});
                else if (text.startsWith('failed')) blocks.push({kind: 'marker', event: e, note: 'failed'});
                break;
            }
            case 'system': {
                const l = last();
                if (l && l.kind === 'setup') l.items.push(e);
                else blocks.push({kind: 'setup', items: [e]});
                break;
            }
            default:
                break;
        }
    }
    return {blocks, responses};
};

const SetupGroup = ({items}: {items: AgentEvent[]}) => {
    const {t} = useTranslation();
    const [open, setOpen] = useState(false);
    return (
        <Box sx={{pl: {xs: 0, sm: 4.5}}}>
            <Typography
                variant="caption"
                color="text.disabled"
                onClick={() => setOpen((v) => !v)}
                sx={{cursor: 'pointer', display: 'inline-flex', alignItems: 'center', gap: 0.5, '&:hover': {color: 'text.secondary'}}}
            >
                {open ? <ExpandMore fontSize="inherit" /> : <ChevronRight fontSize="inherit" />}
                {t('tasks.detail.systemEvents', {count: items.length})}
            </Typography>
            <Collapse in={open} unmountOnExit>
                <Box sx={{pl: 2.5, pt: 0.5}}>
                    {items.map((e) => (
                        <Typography key={e.seq} variant="caption" color="text.secondary" sx={{fontFamily: 'monospace', display: 'block', wordBreak: 'break-all'}}>
                            {e.text}
                        </Typography>
                    ))}
                </Box>
            </Collapse>
        </Box>
    );
};

const EventTimeline = ({events, pending, onRespond}: Props) => {
    const {t} = useTranslation();
    const pendingById = useMemo(() => new Map(pending.map((p) => [p.requestId, p])), [pending]);
    const {blocks, responses} = useMemo(() => toBlocks(events), [events]);

    return (
        <Stack spacing={2}>
            {blocks.map((b, i) => {
                switch (b.kind) {
                    case 'user':
                        return <UserTurn key={b.event.seq} text={b.event.text ?? ''} />;
                    case 'agent':
                        return (
                            <AgentTurn key={b.event.seq}>
                                <RichText text={b.event.text ?? ''} />
                            </AgentTurn>
                        );
                    case 'activity':
                        return <ActivityGroup key={`act-${b.calls[0].use.seq}`} calls={b.calls} />;
                    case 'request':
                        return (
                            <RequestCard
                                key={b.event.seq}
                                event={b.event}
                                pending={pendingById.get(b.event.request_id ?? '')}
                                resolution={responses.get(b.event.request_id ?? '')}
                                onRespond={onRespond}
                            />
                        );
                    case 'error':
                        return (
                            <Box key={b.event.seq} sx={{pl: {xs: 0, sm: 4.5}, pr: {xs: 0, sm: 6}}}>
                                <Typography variant="body2" color="error" sx={{whiteSpace: 'pre-wrap', wordBreak: 'break-word'}}>{b.event.text}</Typography>
                            </Box>
                        );
                    case 'marker':
                        return (
                            <Typography key={b.event.seq} variant="caption" color="text.disabled" sx={{textAlign: 'center'}}>
                                — {b.note === 'interrupted' ? t('tasks.detail.interrupted') : t('tasks.detail.failedMarker')} —
                            </Typography>
                        );
                    case 'setup':
                        return <SetupGroup key={`setup-${i}`} items={b.items} />;
                    default:
                        return <Fragment key={i} />;
                }
            })}
        </Stack>
    );
};

export default EventTimeline;
