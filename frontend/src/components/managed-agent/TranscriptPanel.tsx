import {Archive, PlayerStop, Send} from '@/components/icons';
import UnifiedCard from '@/components/UnifiedCard';
import EmptyState from '@/components/EmptyState';
import type {MessageInfo, SessionInfo} from '@/services/managedAgentApi';
import {findPendingRequest, isBusyStatus, STATUS_COLOR} from './managedAgentUtils';
import MessageItem from './MessageItem';
import PermissionModeSelect from './PermissionModeSelect';
import {
    Alert,
    Box,
    Button,
    Chip,
    IconButton,
    Stack,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import {useEffect, useMemo, useRef, useState} from 'react';
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
}

// TranscriptPanel is the "what's happening now, what do I do next" surface
// (ux-principles #1): status + the one actionable request are always
// visible without scrolling back through history, and the composer itself
// explains why it's disabled instead of just going inert.
const TranscriptPanel = ({
    session, messages, permissionModes, onSend, onRespond, onInterrupt, onArchive, onPermissionModeChange,
}: TranscriptPanelProps) => {
    const {t} = useTranslation();
    const [text, setText] = useState('');
    const [sending, setSending] = useState(false);
    const [responding, setResponding] = useState(false);
    const bottomRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        bottomRef.current?.scrollIntoView({block: 'end'});
    }, [messages.length]);

    const pendingRequest = useMemo(() => findPendingRequest(messages), [messages]);
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

    return (
        <UnifiedCard size="full" titleMarginBottom={1}
            title={(
                <Stack direction="row" spacing={1} sx={{alignItems: "center"}}>
                    <Typography component="span" sx={{fontFamily: 'monospace', fontSize: '0.95rem'}}>{session.project}</Typography>
                    <Chip size="small" label={session.status} color={STATUS_COLOR[session.status] || 'default'}/>
                </Stack>
            )}
            rightAction={(
                <Stack direction="row" spacing={1}>
                    <PermissionModeSelect value={session.permission_mode} permissionModes={permissionModes} onChange={onPermissionModeChange}/>
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
    );
};

export default TranscriptPanel;
