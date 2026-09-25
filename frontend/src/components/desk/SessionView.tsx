import {Archive, ArrowBack, Check, Close, ContentCopy, FoldUp, Terminal, UnfoldMore} from '@/components/icons';
import {useCopyFeedback} from '@/hooks/useCopyFeedback';
import type {MessageInfo, SessionInfo} from '@/services/deskApi';
import {Alert, Box, Button, Chip, IconButton, Stack, Tooltip, Typography} from '@mui/material';
import {useEffect, useMemo, useRef, useState} from 'react';
import {useTranslation} from 'react-i18next';
import Composer from './Composer';
import {buildTranscript, isBusyStatus, pendingRequestId, sessionTitle} from './deskUtils';
import FolderChip from './FolderChip';
import ModelSelect from './ModelSelect';
import PermissionModeSelect from './PermissionModeSelect';
import ProfileSelect from './ProfileSelect';
import StatusLine from './StatusLine';
import Transcript from './Transcript';

interface SessionViewProps {
    session: SessionInfo;
    messages: MessageInfo[];
    permissionModes: string[];
    onSend: (text: string) => Promise<boolean>;
    onRespond: (requestId: string, approved: boolean, answer: string) => Promise<void>;
    onInterrupt: () => Promise<void>;
    onArchive: () => Promise<void>;
    onPermissionModeChange: (mode: string) => Promise<void>;
    onProfileChange: (profile: string) => Promise<void>;
    onModelChange: (model: string) => Promise<void>;
    // Follow-ups typed while a turn runs; they are sent together once it
    // ends. Taking one back puts its text into the draft.
    queued: string[];
    onUnqueue: (index: number) => void;
    onSendQueuedNow: () => void;
    draft: string;
    onDraftChange: (text: string) => void;
    // Releases the session to a local terminal; resolves the command to run,
    // or null if it couldn't.
    onHandoff: () => Promise<string | null>;
    // Set on narrow screens, where the session list is a separate view.
    onBack?: () => void;
}

const COLUMN_MAX_WIDTH = 760;
const EXPAND_KEY = 'desk.expandTools';

const readExpand = () => {
    try {
        return localStorage.getItem(EXPAND_KEY) === '1';
    } catch {
        return false;
    }
};

const SessionView = ({
    session, messages, permissionModes, onSend, onRespond, onInterrupt, onArchive, onPermissionModeChange, onProfileChange, onModelChange,
    queued, onUnqueue, onSendQueuedNow, draft, onDraftChange, onHandoff, onBack,
}: SessionViewProps) => {
    const {t} = useTranslation();
    const scrollRef = useRef<HTMLDivElement>(null);
    const [expandAll, setExpandAll] = useState(readExpand);
    const toggleExpand = () => {
        const next = !expandAll;
        setExpandAll(next);
        try {
            localStorage.setItem(EXPAND_KEY, next ? '1' : '0');
        } catch {
            // Only a remembered preference; the toggle still works.
        }
    };

    // The handoff command stays on screen (with its own copy button) since
    // copying right after the request can be refused by the browser.
    // Keyed by session so it disappears when another session is opened.
    const [handoffResult, setHandoffResult] = useState<{id: string; command: string} | null>(null);
    const handoffCommand = handoffResult?.id === session.id ? handoffResult.command : null;
    const setHandoffCommand = (command: string | null) => setHandoffResult(command ? {id: session.id, command} : null);
    const {copied, copy} = useCopyFeedback();
    const handoff = async () => {
        const cmd = await onHandoff();
        if (!cmd) return;
        setHandoffCommand(cmd);
        copy(cmd);
    };

    const blocks = useMemo(() => buildTranscript(messages), [messages]);
    const turnInFlight = isBusyStatus(session.status);
    const pendingId = pendingRequestId(blocks, turnInFlight);
    const isClosed = session.status === 'closed';

    // Follow new output, but only if the reader is already at the bottom:
    // scrolling up to read earlier output must not be yanked back down.
    const stickToBottom = useRef(true);
    useEffect(() => {
        const el = scrollRef.current;
        if (el && stickToBottom.current) el.scrollTop = el.scrollHeight;
    }, [blocks]);
    useEffect(() => {
        stickToBottom.current = true;
    }, [session.id]);

    return (
        <Box sx={{display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0}}>
            <Stack
                direction="row"
                spacing={1}
                sx={{alignItems: 'center', px: 2, py: 1.25, borderBottom: 1, borderColor: 'divider', minWidth: 0}}
            >
                {onBack && (
                    <IconButton size="small" onClick={onBack} aria-label={t('common.back', {defaultValue: 'Back'})}>
                        <ArrowBack fontSize="small"/>
                    </IconButton>
                )}
                <Typography variant="subtitle1" noWrap sx={{fontWeight: 600, minWidth: 0, color: 'text.primary'}}>{sessionTitle(session)}</Typography>
                <FolderChip path={session.project}/>
                {session.status === 'failed' && <Chip size="small" color="error" variant="outlined" label={t('desk.statusFailed', {defaultValue: 'failed'})}/>}
                {isClosed && <Chip size="small" variant="outlined" label={t('desk.statusArchived', {defaultValue: 'archived'})}/>}
                <Box sx={{flex: 1}}/>
                <Tooltip title={expandAll
                    ? t('desk.collapseTools', {defaultValue: 'Collapse tool calls'})
                    : t('desk.expandTools', {defaultValue: 'Expand all tool calls'})}
                >
                    <IconButton size="small" onClick={toggleExpand} aria-label={t('desk.expandTools', {defaultValue: 'Expand all tool calls'})} aria-pressed={expandAll}>
                        {expandAll ? <FoldUp fontSize="small"/> : <UnfoldMore fontSize="small"/>}
                    </IconButton>
                </Tooltip>
                {!isClosed && (
                    <Tooltip title={turnInFlight
                        ? t('desk.handoffBusy', {defaultValue: 'Stop or wait for the current turn to continue in a terminal'})
                        : t('desk.handoffHint', {defaultValue: 'Continue in terminal — copies a command that resumes this session through tingly-box'})}
                    >
                        {/* span: a disabled button fires no events for the tooltip */}
                        <span>
                            <IconButton size="small" disabled={turnInFlight} onClick={() => void handoff()} aria-label={t('desk.handoff', {defaultValue: 'Continue in terminal'})}>
                                <Terminal fontSize="small"/>
                            </IconButton>
                        </span>
                    </Tooltip>
                )}
                {!isClosed && (
                    <Tooltip title={t('desk.archiveHint', {defaultValue: 'Archive — ends the session; the folder and history stay'})}>
                        <IconButton size="small" onClick={onArchive} aria-label={t('desk.archive', {defaultValue: 'Archive'})}><Archive fontSize="small"/></IconButton>
                    </Tooltip>
                )}
            </Stack>

            <Box
                ref={scrollRef}
                onScroll={(e) => {
                    const el = e.currentTarget;
                    stickToBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
                }}
                sx={{flex: 1, minHeight: 0, overflowY: 'auto', px: 2}}
            >
                <Box sx={{maxWidth: COLUMN_MAX_WIDTH, mx: 'auto', py: 3}}>
                    <Transcript blocks={blocks} pendingRequestId={pendingId} working={turnInFlight} expandAll={expandAll} onRespond={onRespond}/>
                </Box>
            </Box>

            <Box sx={{px: 2, pb: 2, pt: 1}}>
                <Box sx={{maxWidth: COLUMN_MAX_WIDTH, mx: 'auto'}}>
                    {handoffCommand && (
                        <Alert
                            severity="info"
                            variant="outlined"
                            icon={<Terminal fontSize="small"/>}
                            onClose={() => setHandoffCommand(null)}
                            sx={{mb: 1, '& .MuiAlert-message': {minWidth: 0, flex: 1}}}
                        >
                            <Typography variant="body2" sx={{color: 'text.primary', mb: 0.5}}>
                                {t('desk.handoffReady', {defaultValue: 'Run this in a terminal on this machine. Use one place at a time — a message sent here starts the session again.'})}
                            </Typography>
                            <Stack direction="row" spacing={0.5} sx={{alignItems: 'center'}}>
                                <Box
                                    component="code"
                                    sx={{
                                        flex: 1, minWidth: 0, overflowX: 'auto', whiteSpace: 'nowrap', fontFamily: 'monospace', fontSize: '0.8rem',
                                        px: 1, py: 0.5, borderRadius: 1, bgcolor: 'action.hover', color: 'text.primary',
                                    }}
                                >
                                    {handoffCommand}
                                </Box>
                                <Tooltip title={copied ? t('common.copied', {defaultValue: 'Copied'}) : t('common.copy', {defaultValue: 'Copy'})}>
                                    <IconButton size="small" onClick={() => copy(handoffCommand)} aria-label={t('common.copy', {defaultValue: 'Copy'})}>
                                        {copied ? <Check fontSize="small"/> : <ContentCopy fontSize="small"/>}
                                    </IconButton>
                                </Tooltip>
                            </Stack>
                        </Alert>
                    )}
                    {!isClosed && queued.length > 0 && (
                        <Stack spacing={0.5} sx={{mb: 1}}>
                            <Stack direction="row" sx={{alignItems: 'center'}}>
                                <Typography variant="caption" sx={{color: 'text.secondary', flex: 1}}>
                                    {turnInFlight
                                        ? t('desk.queuedHint', {defaultValue: 'Queued — sent when the current turn ends'})
                                        : t('desk.queuedHeld', {defaultValue: 'Not sent — the last turn didn’t complete'})}
                                </Typography>
                                {!turnInFlight && (
                                    <Button size="small" onClick={onSendQueuedNow}>{t('desk.sendNow', {defaultValue: 'Send now'})}</Button>
                                )}
                            </Stack>
                            {queued.map((q, i) => (
                                <Stack
                                    key={i}
                                    direction="row"
                                    spacing={1}
                                    sx={{alignItems: 'flex-start', px: 1.5, py: 0.75, borderRadius: 2, border: 1, borderStyle: 'dashed', borderColor: 'divider'}}
                                >
                                    <Typography variant="body2" sx={{flex: 1, minWidth: 0, color: 'text.primary', whiteSpace: 'pre-wrap', wordBreak: 'break-word'}}>{q}</Typography>
                                    <Tooltip title={t('desk.unqueue', {defaultValue: 'Take back into the input'})}>
                                        <IconButton size="small" sx={{mt: -0.25}} onClick={() => onUnqueue(i)} aria-label={t('desk.unqueue', {defaultValue: 'Take back into the input'})}>
                                            <Close sx={{fontSize: 16}}/>
                                        </IconButton>
                                    </Tooltip>
                                </Stack>
                            ))}
                        </Stack>
                    )}
                    {isClosed ? (
                        <Alert severity="info" variant="outlined">
                            {t('desk.archived', {defaultValue: 'This session is archived — the folder and history stay, but it can no longer be steered.'})}
                        </Alert>
                    ) : (
                        <Composer
                            key={session.id}
                            text={draft}
                            onTextChange={onDraftChange}
                            onStop={turnInFlight ? onInterrupt : undefined}
                            placeholder={turnInFlight
                                ? (pendingId
                                    ? t('desk.waitingForYou', {defaultValue: 'Waiting for your answer above…'})
                                    : t('desk.queuePlaceholder', {defaultValue: 'Queue a follow-up…'}))
                                : t('desk.messagePlaceholder', {defaultValue: 'Reply…'})}
                            onSubmit={onSend}
                            minRows={1}
                            context={(
                                <>
                                    <ProfileSelect value={session.profile} onChange={(p) => void onProfileChange(p)}/>
                                    <ModelSelect profile={session.profile} value={session.model} onChange={(m) => void onModelChange(m)}/>
                                    <PermissionModeSelect
                                        value={session.permission_mode}
                                        permissionModes={permissionModes}
                                        onChange={(m) => void onPermissionModeChange(m)}
                                    />
                                </>
                            )}
                        />
                    )}
                    <StatusLine sessionId={session.id} profile={session.profile} model={session.model} messages={messages}/>
                </Box>
            </Box>
        </Box>
    );
};

export default SessionView;
