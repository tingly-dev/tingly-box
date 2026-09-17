import {Archive, ExpandMore, GitCompare, PlayerStop, Send} from '@/components/icons';
import {CopyIconButton} from '@/components/CopyIconButton';
import UnifiedCard from '@/components/UnifiedCard';
import EmptyState from '@/components/EmptyState';
import type {Diff, MessageInfo, SessionInfo} from '@/services/managedAgentApi';
import {findPendingRequest, isBusyStatus, STATUS_COLOR} from './managedAgentUtils';
import MessageItem from './MessageItem';
import {
    Accordion,
    AccordionDetails,
    AccordionSummary,
    Alert,
    Box,
    Button,
    Chip,
    IconButton,
    MenuItem,
    Stack,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import {useEffect, useRef, useState} from 'react';
import {useTranslation} from 'react-i18next';

interface TranscriptPanelProps {
    session: SessionInfo;
    messages: MessageInfo[];
    permissionModes: string[];
    onSend: (text: string) => Promise<void>;
    onRespond: (requestId: string, approved: boolean, answer: string) => Promise<void>;
    onInterrupt: () => Promise<void>;
    onArchive: () => Promise<void>;
    onPermissionModeChange: (mode: string) => Promise<void>;
    loadDiff: () => Promise<Diff>;
}

// TranscriptPanel is the "what's happening now, what do I do next" surface
// (ux-principles #1): status + the one actionable request are always
// visible without scrolling back through history, and the composer itself
// explains why it's disabled instead of just going inert.
const TranscriptPanel = ({
    session, messages, permissionModes, onSend, onRespond, onInterrupt, onArchive, onPermissionModeChange, loadDiff,
}: TranscriptPanelProps) => {
    const {t} = useTranslation();
    const [text, setText] = useState('');
    const [sending, setSending] = useState(false);
    const [responding, setResponding] = useState(false);
    const [diff, setDiff] = useState<Diff | null>(null);
    const [diffOpen, setDiffOpen] = useState(false);
    const [diffLoading, setDiffLoading] = useState(false);
    const bottomRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        bottomRef.current?.scrollIntoView({block: 'end'});
    }, [messages.length]);

    const pendingRequest = findPendingRequest(messages);
    const isClosed = session.status === 'closed';
    const turnInFlight = isBusyStatus(session.status);
    const canSend = !isClosed && !turnInFlight && text.trim() !== '' && !sending;

    const handleSend = async () => {
        if (!canSend) return;
        setSending(true);
        try {
            await onSend(text.trim());
            setText('');
        } finally {
            setSending(false);
        }
    };

    const handleRespond = async (approved: boolean, answer: string) => {
        if (!pendingRequest?.request_id) return;
        setResponding(true);
        try {
            await onRespond(pendingRequest.request_id, approved, answer);
        } finally {
            setResponding(false);
        }
    };

    const handleDiffToggle = async () => {
        const next = !diffOpen;
        setDiffOpen(next);
        if (next && !diff) {
            setDiffLoading(true);
            try {
                setDiff(await loadDiff());
            } finally {
                setDiffLoading(false);
            }
        }
    };

    return (
        <Stack spacing={2} sx={{height: '100%'}}>
            <UnifiedCard size="full" titleMarginBottom={1}
                title={(
                    <Stack direction="row" spacing={1} sx={{alignItems: "center"}}>
                        <Typography component="span" sx={{fontFamily: 'monospace', fontSize: '0.95rem'}}>{session.project}</Typography>
                        <Chip size="small" label={session.status} color={STATUS_COLOR[session.status] || 'default'}/>
                    </Stack>
                )}
                rightAction={(
                    <Stack direction="row" spacing={1}>
                        <TextField
                            select
                            size="small"
                            label={t('managedAgent.permissionMode', {defaultValue: 'Permission'})}
                            value={session.permission_mode}
                            onChange={(e) => onPermissionModeChange(e.target.value)}
                            sx={{minWidth: 160}}
                        >
                            <MenuItem value="">{t('managedAgent.permissionInherit', {defaultValue: 'Inherit (default)'})}</MenuItem>
                            {permissionModes.map((m) => <MenuItem key={m} value={m}>{m}</MenuItem>)}
                        </TextField>
                        {turnInFlight && (
                            <Tooltip title={t('managedAgent.interrupt', {defaultValue: 'Stop the current turn'})}>
                                <Button size="small" color="warning" variant="outlined" startIcon={<PlayerStop/>} onClick={onInterrupt}>
                                    {t('managedAgent.interruptShort', {defaultValue: 'Stop'})}
                                </Button>
                            </Tooltip>
                        )}
                        {!isClosed && (
                            <Tooltip title={t('managedAgent.archiveHint', {defaultValue: 'End this session for good — the folder and history stay'})}>
                                <IconButton size="small" onClick={onArchive}><Archive fontSize="small"/></IconButton>
                            </Tooltip>
                        )}
                    </Stack>
                )}
            >
                {session.error && <Alert severity="error" sx={{mb: 1}}>{session.error}</Alert>}

                <Box sx={{display: 'flex', flexDirection: 'column', gap: 1.25, minHeight: 240, maxHeight: '55vh', overflowY: 'auto', p: 0.5}}>
                    {messages.length === 0 ? (
                        <EmptyState compact title={t('managedAgent.noMessages', {defaultValue: 'No messages yet'})}/>
                    ) : (
                        messages.map((m, i) => (
                            <MessageItem
                                key={`${m.timestamp}-${i}`}
                                message={m}
                                pending={Boolean(pendingRequest) && m === pendingRequest}
                                onRespond={handleRespond}
                                responding={responding}
                            />
                        ))
                    )}
                    <div ref={bottomRef}/>
                </Box>

                <Box sx={{mt: 1.5}}>
                    {isClosed ? (
                        <Alert severity="info" variant="outlined">
                            {t('managedAgent.archived', {defaultValue: 'This session is archived — the folder and history stay, but it can no longer be steered.'})}
                        </Alert>
                    ) : (
                        <Stack direction="row" spacing={1} sx={{alignItems: "flex-end"}}>
                            <TextField
                                multiline
                                minRows={1}
                                maxRows={6}
                                fullWidth
                                size="small"
                                placeholder={turnInFlight
                                    ? t('managedAgent.turnInProgress', {defaultValue: 'A turn is in progress…'})
                                    : t('managedAgent.messagePlaceholder', {defaultValue: 'Send a message'})}
                                value={text}
                                disabled={turnInFlight}
                                onChange={(e) => setText(e.target.value)}
                                onKeyDown={(e) => {
                                    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                                        e.preventDefault();
                                        void handleSend();
                                    }
                                }}
                            />
                            <Button variant="contained" disabled={!canSend} onClick={handleSend} startIcon={<Send/>}>
                                {t('managedAgent.send', {defaultValue: 'Send'})}
                            </Button>
                        </Stack>
                    )}
                </Box>
            </UnifiedCard>

            <Accordion expanded={diffOpen} onChange={handleDiffToggle} disableGutters variant="outlined">
                <AccordionSummary expandIcon={<ExpandMore/>}>
                    <Stack direction="row" spacing={1} sx={{alignItems: "center"}}>
                        <GitCompare fontSize="small"/>
                        <Typography variant="subtitle2">{t('managedAgent.changes', {defaultValue: 'Changes'})}</Typography>
                        {diff && diff.changed_files > 0 && <Chip size="small" label={diff.changed_files}/>}
                    </Stack>
                </AccordionSummary>
                <AccordionDetails>
                    {diffLoading && <Typography variant="body2" color="text.secondary">{t('managedAgent.loading', {defaultValue: 'Loading…'})}</Typography>}
                    {!diffLoading && diff && diff.changed_files === 0 && (
                        <Typography variant="body2" color="text.secondary">{t('managedAgent.noChanges', {defaultValue: 'No changes in this folder.'})}</Typography>
                    )}
                    {!diffLoading && diff && diff.changed_files > 0 && (
                        <Box>
                            <Typography component="pre" variant="caption" sx={{fontFamily: 'monospace', whiteSpace: 'pre-wrap', color: 'text.secondary'}}>
                                {diff.stat}
                            </Typography>
                            {diff.untracked && diff.untracked.length > 0 && (
                                <Typography variant="caption" color="text.secondary" sx={{display: 'block', mt: 0.5}}>
                                    {t('managedAgent.untracked', {defaultValue: 'Untracked: {{files}}', files: diff.untracked.join(', ')})}
                                </Typography>
                            )}
                            {diff.patch && (
                                <Box sx={{position: 'relative', mt: 1}}>
                                    <CopyIconButton value={diff.patch} sx={{position: 'absolute', top: 4, right: 4}}/>
                                    <Box component="pre" sx={{
                                        fontFamily: 'monospace', fontSize: '0.75rem', whiteSpace: 'pre-wrap',
                                        maxHeight: 400, overflow: 'auto', bgcolor: 'action.hover', p: 1.5, borderRadius: 1, m: 0,
                                    }}>
                                        {diff.patch}{diff.truncated ? '\n… (truncated)' : ''}
                                    </Box>
                                </Box>
                            )}
                        </Box>
                    )}
                </AccordionDetails>
            </Accordion>
        </Stack>
    );
};

export default TranscriptPanel;
