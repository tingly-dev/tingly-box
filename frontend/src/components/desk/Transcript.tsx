import {Check, ChevronRight, Close, ErrorOutline, ExpandMore} from '@/components/icons';
import type {MessageInfo} from '@/services/deskApi';
import {Box, Button, CircularProgress, Collapse, Paper, Stack, TextField, Typography} from '@mui/material';
import {useState} from 'react';
import {useTranslation} from 'react-i18next';
import type {ActivityStep, TranscriptBlock} from './deskUtils';
import {toolSummary} from './deskUtils';

interface TranscriptProps {
    blocks: TranscriptBlock[];
    pendingRequestId?: string;
    working: boolean;
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

const AssistantText = ({message}: {message: MessageInfo}) => (
    <Typography variant="body1" sx={conversationText}>
        {message.content}
    </Typography>
);

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
const ActivityRow = ({steps, live}: {steps: ActivityStep[]; live: boolean}) => {
    const {t} = useTranslation();
    const [open, setOpen] = useState(false);
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

const Transcript = ({blocks, pendingRequestId, working, onRespond}: TranscriptProps) => {
    const {t} = useTranslation();
    const last = blocks[blocks.length - 1];
    // The spinner rides on the last activity row while a turn runs; if the
    // turn hasn't produced any activity yet, it gets a line of its own.
    const showWorking = working && !pendingRequestId && last?.type !== 'activity';

    return (
        <Stack spacing={2.5}>
            {blocks.map((b, i) => {
                switch (b.type) {
                    case 'user':
                        return <UserBubble key={i} message={b.message}/>;
                    case 'assistant':
                        return <AssistantText key={i} message={b.message}/>;
                    case 'activity':
                        return <ActivityRow key={i} steps={b.steps} live={working && i === blocks.length - 1}/>;
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
