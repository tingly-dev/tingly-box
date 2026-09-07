// The conversation view of a task's event log. Assistant and user turns are
// the anchors; tool activity is folded behind a toggle so a 200-event run
// reads as a conversation, not a log; approval / ask requests render as
// cards with the answer controls inline (the one thing that must not be
// missed, especially on a phone).
import {Fragment, useMemo, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {
    Box, Button, Chip, Collapse, Paper, Stack, TextField, Typography,
} from '@mui/material';
import {alpha} from '@mui/material/styles';
import {ExpandLess, ExpandMore} from '@/components/icons';
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

const briefInput = (input: unknown): string => {
    if (!input || typeof input !== 'object') return '';
    const m = input as Record<string, unknown>;
    const pick = m.command ?? m.file_path ?? m.path ?? m.pattern ?? m.url ?? m.description;
    const s = typeof pick === 'string' ? pick : JSON.stringify(m);
    return s.length > 120 ? `${s.slice(0, 117)}…` : s;
};

const Bubble = ({role, children}: {role: 'user' | 'assistant'; children: React.ReactNode}) => (
    <Box sx={{display: 'flex', justifyContent: role === 'user' ? 'flex-end' : 'flex-start'}}>
        <Paper
            variant="outlined"
            sx={(theme) => ({
                px: 1.75, py: 1.25, maxWidth: {xs: '92%', md: '80%'},
                bgcolor: role === 'user' ? alpha(theme.palette.primary.main, 0.08) : 'background.paper',
                borderColor: role === 'user' ? alpha(theme.palette.primary.main, 0.3) : 'divider',
                whiteSpace: 'pre-wrap', wordBreak: 'break-word',
            })}
        >
            <Typography variant="body2">{children}</Typography>
        </Paper>
    </Box>
);

const RequestCard = ({event, pending, onRespond}: {event: AgentEvent; pending?: PendingRequest; onRespond: Props['onRespond']}) => {
    const {t} = useTranslation();
    const [answer, setAnswer] = useState('');
    const [busy, setBusy] = useState(false);
    const payload = payloadOf(event);
    const isAsk = event.kind === 'ask_request';
    const title = isAsk
        ? t('tasks.detail.askTitle')
        : t('tasks.detail.approvalTitle', {tool: event.text || (payload.tool as string) || 'tool'});
    const detail = isAsk ? (event.text || (payload.message as string) || '') : briefInput(payload.input);

    const act = async (approved: boolean) => {
        setBusy(true);
        try {
            await onRespond(event.request_id ?? '', approved, isAsk ? answer : undefined);
        } finally {
            setBusy(false);
        }
    };

    return (
        <Paper
            variant="outlined"
            sx={(theme) => ({
                p: 2,
                borderColor: pending ? theme.palette.warning.main : 'divider',
                bgcolor: pending ? alpha(theme.palette.warning.main, 0.06) : 'background.paper',
            })}
        >
            <Stack spacing={1.25}>
                <Typography variant="subtitle2">{title}</Typography>
                {detail && (
                    <Typography variant="body2" sx={{fontFamily: isAsk ? undefined : 'monospace', whiteSpace: 'pre-wrap', wordBreak: 'break-word'}}>
                        {detail}
                    </Typography>
                )}
                {pending && isAsk && (
                    <TextField
                        size="small"
                        fullWidth
                        multiline
                        maxRows={4}
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
                                <Button variant="contained" color="success" disabled={busy} onClick={() => act(true)}>
                                    {t('tasks.detail.approve')}
                                </Button>
                                <Button variant="outlined" color="error" disabled={busy} onClick={() => act(false)}>
                                    {t('tasks.detail.deny')}
                                </Button>
                            </>
                        )}
                    </Stack>
                )}
            </Stack>
        </Paper>
    );
};

const ToolRow = ({event}: {event: AgentEvent}) => {
    const {t} = useTranslation();
    const payload = payloadOf(event);
    if (event.kind === 'tool_use') {
        return (
            <Typography variant="caption" color="text.secondary" sx={{fontFamily: 'monospace', display: 'block', wordBreak: 'break-all'}}>
                ▸ {t('tasks.detail.toolUse', {name: event.text})} {briefInput(payload)}
            </Typography>
        );
    }
    const isError = payload.is_error === true;
    return (
        <Typography variant="caption" color={isError ? 'error' : 'text.secondary'} sx={{fontFamily: 'monospace', display: 'block', whiteSpace: 'pre-wrap', wordBreak: 'break-word', pl: 1.5}}>
            {isError ? t('tasks.detail.toolError') : t('tasks.detail.toolResult')}: {(event.text ?? '').slice(0, 300)}
        </Typography>
    );
};

const EventTimeline = ({events, pending, onRespond}: Props) => {
    const {t} = useTranslation();
    const [showTools, setShowTools] = useState(false);
    const [showSetup, setShowSetup] = useState(false);
    const pendingById = useMemo(() => new Map(pending.map((p) => [p.requestId, p])), [pending]);

    // Group consecutive setup (system) lines so provisioning does not
    // dominate the top of the conversation.
    const groups = useMemo(() => {
        const out: Array<{kind: 'system'; items: AgentEvent[]} | {kind: 'event'; item: AgentEvent}> = [];
        for (const e of events) {
            if (e.kind === 'system') {
                const last = out[out.length - 1];
                if (last && last.kind === 'system') last.items.push(e);
                else out.push({kind: 'system', items: [e]});
            } else {
                out.push({kind: 'event', item: e});
            }
        }
        return out;
    }, [events]);

    const toolCount = events.filter((e) => e.kind === 'tool_use' || e.kind === 'tool_result').length;

    return (
        <Stack spacing={1.5}>
            {toolCount > 0 && (
                <Box>
                    <Chip
                        size="small"
                        variant="outlined"
                        onClick={() => setShowTools((v) => !v)}
                        icon={showTools ? <ExpandLess /> : <ExpandMore />}
                        label={`${showTools ? t('tasks.detail.hideTools') : t('tasks.detail.showTools')} (${toolCount})`}
                    />
                </Box>
            )}
            {groups.map((g, i) => {
                if (g.kind === 'system') {
                    return (
                        <Box key={`sys-${i}`}>
                            <Chip
                                size="small"
                                variant="outlined"
                                onClick={() => setShowSetup((v) => !v)}
                                icon={showSetup ? <ExpandLess /> : <ExpandMore />}
                                label={t('tasks.detail.systemEvents', {count: g.items.length})}
                            />
                            <Collapse in={showSetup}>
                                <Box sx={{pl: 1, pt: 0.5}}>
                                    {g.items.map((e) => (
                                        <Typography key={e.seq} variant="caption" color="text.secondary" sx={{fontFamily: 'monospace', display: 'block', wordBreak: 'break-all'}}>
                                            {e.text}
                                        </Typography>
                                    ))}
                                </Box>
                            </Collapse>
                        </Box>
                    );
                }
                const e = g.item;
                switch (e.kind) {
                    case 'user_message':
                        return <Bubble key={e.seq} role="user">{e.text}</Bubble>;
                    case 'assistant_message':
                        return <Bubble key={e.seq} role="assistant">{e.text}</Bubble>;
                    case 'approval_request':
                    case 'ask_request':
                        return <RequestCard key={e.seq} event={e} pending={pendingById.get(e.request_id ?? '')} onRespond={onRespond} />;
                    case 'approval_response':
                    case 'ask_response':
                        return (
                            <Typography key={e.seq} variant="caption" color="text.secondary" sx={{textAlign: 'right'}}>
                                ↳ {e.text}
                            </Typography>
                        );
                    case 'tool_use':
                    case 'tool_result':
                        return showTools ? <ToolRow key={e.seq} event={e} /> : <Fragment key={e.seq} />;
                    case 'error':
                        return (
                            <Typography key={e.seq} variant="body2" color="error" sx={{whiteSpace: 'pre-wrap'}}>
                                {e.text}
                            </Typography>
                        );
                    case 'status':
                        return (
                            <Typography key={e.seq} variant="caption" color="text.disabled" sx={{textAlign: 'center'}}>
                                — {t(`tasks.status.${(e.text ?? '').split(':')[0]}`, {defaultValue: e.text})} —
                            </Typography>
                        );
                    default:
                        return <Fragment key={e.seq} />;
                }
            })}
        </Stack>
    );
};

export default EventTimeline;
