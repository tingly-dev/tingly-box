import {Archive, ArrowBack, KeyboardArrowDown, MoreVert, Check, Close, ContentCopy, FoldUp, Stream, Terminal, UnfoldMore} from '@/components/icons';
import ConfirmDialog from '@/components/ConfirmDialog';
import {useCopyFeedback} from '@/hooks/useCopyFeedback';
import type {MessageInfo, SessionInfo} from '@/services/deskApi';
import {Alert, Box, Button, Chip, IconButton, Popover, Stack, ToggleButton, ToggleButtonGroup, Tooltip, Typography, Menu, MenuItem, LinearProgress} from '@mui/material';
import {useEffect, useMemo, useRef, useState} from 'react';
import {useTranslation} from 'react-i18next';
import BackgroundTasksPanel from './BackgroundTasksPanel';
import Composer from './Composer';
import {backgroundTasks, buildTranscript, isBusyStatus, pendingRequestId, sessionTitle} from './deskUtils';
import FolderChip from './FolderChip';
import ModelSelect from './ModelSelect';
import PermissionModeSelect from './PermissionModeSelect';
import ProfileSelect from './ProfileSelect';
import StatusLine from './StatusLine';
import Trajectory from './Trajectory';
import type {TrajectoryAnchor} from './trajectoryRows';
import Transcript from './Transcript';
import { fontMono, fontSizes } from '@/theme/fonts';
import {getReadableAccent} from '@/theme/status';
import {useTranscriptScroll} from './useTranscriptScroll';
import type {DeskQueue} from './useDeskQueues';

interface SessionViewProps {
    session: SessionInfo;
    messages: MessageInfo[];
    permissionModes: string[];
    onSend: (text: string) => Promise<boolean>;
    onRespond: (requestId: string, approved: boolean, answer: string) => Promise<boolean>;
    onInterrupt: () => Promise<void>;
    onArchive: () => Promise<boolean>;
    onPermissionModeChange: (mode: string) => Promise<void>;
    onProfileChange: (profile: string) => Promise<void>;
    onModelChange: (model: string) => Promise<void>;
    // Follow-ups typed while a turn runs; they are sent together once it
    // ends. Taking one back puts its text into the draft.
    queued: string[];
    queueHeld?: DeskQueue['held'];
    queueSending?: boolean;
    messagesLoading?: boolean;
    onUnqueue: (index: number) => void;
    onSendQueuedNow: () => void;
    draft: string;
    onDraftChange: (text: string) => void;
    onDraftAccepted: (submitted: string) => void;
    // Releases the session to a local terminal; resolves the command to run,
    // or null if it couldn't.
    onHandoff: () => Promise<string | null>;
    // Re-reads the transcript and the session now, e.g. after stopping a task.
    onRefresh: () => void;
    // Set on narrow screens, where the session list is a separate view.
    onBack?: () => void;
}

const COLUMN_MAX_WIDTH = 760;
const EXPAND_KEY = 'desk.expandTools';

const VIEW_KEY = 'desk.view';
type SessionViewMode = 'chat' | 'trajectory';

const readView = (): SessionViewMode => {
    try {
        return localStorage.getItem(VIEW_KEY) === 'trajectory' ? 'trajectory' : 'chat';
    } catch {
        return 'chat';
    }
};

const anchorSelector = (anchor: TrajectoryAnchor): string => {
    if ('call' in anchor) return `[data-call-ids~="${CSS.escape(anchor.call)}"]`;
    if ('request' in anchor) return `[data-request-id="${CSS.escape(anchor.request)}"]`;
    return `[data-block="${anchor.block}"]`;
};

const readExpand = () => {
    try {
        return localStorage.getItem(EXPAND_KEY) === '1';
    } catch {
        return false;
    }
};

const SessionView = ({
    session, messages, permissionModes, onSend, onRespond, onInterrupt, onArchive, onPermissionModeChange, onProfileChange, onModelChange,
    queued, queueHeld, queueSending, messagesLoading, onUnqueue, onSendQueuedNow, draft, onDraftChange, onDraftAccepted, onHandoff, onRefresh, onBack,
}: SessionViewProps) => {
    const {t} = useTranslation();
    const {viewport: scrollRef, content: contentRef, onScroll, latest, reveal: revealElement, away, unread} = useTranscriptScroll(`${messages.length}:${messages.at(-1)?.timestamp ?? ''}:${messages.at(-1)?.content ?? ''}`);
    const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null);
    const [acting, setActing] = useState(false);
    const actingRef = useRef(false);
    const [changingSettings, setChangingSettings] = useState(false);
    const settingsRef = useRef(false);
    const changeSetting = async (run: () => Promise<void>) => {
        if (settingsRef.current) return;
        settingsRef.current = true;
        setChangingSettings(true);
        try {await run();} finally {settingsRef.current = false; setChangingSettings(false);}
    };
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

    // Chat and Trajectory read the same transcript; the choice is remembered.
    const [view, setViewState] = useState<SessionViewMode>(readView);
    const setView = (next: SessionViewMode) => {
        setViewState(next);
        try {
            localStorage.setItem(VIEW_KEY, next);
        } catch {
            // Only a remembered preference; the switch still works.
        }
    };
    // A trajectory row opens the chat at its message once the chat is back.
    const [target, setTarget] = useState<TrajectoryAnchor | null>(null);
    const openInChat = (anchor: TrajectoryAnchor) => {
        setView('chat');
        setTarget(anchor);
    };

    // The handoff command stays on screen (with its own copy button) since
    // copying right after the request can be refused by the browser.
    // Keyed by session so it disappears when another session is opened.
    const [handoffResult, setHandoffResult] = useState<{id: string; command: string} | null>(null);
    const handoffCommand = handoffResult?.id === session.id ? handoffResult.command : null;
    const setHandoffCommand = (command: string | null) => setHandoffResult(command ? {id: session.id, command} : null);
    const {copied, copy} = useCopyFeedback();
    const handoff = async (): Promise<boolean> => {
        const cmd = await onHandoff();
        if (!cmd) return false;
        setHandoffCommand(cmd);
        copy(cmd);
        return true;
    };
    const runAction = async (action: 'archive' | 'handoff') => {
        if (actingRef.current) return;
        actingRef.current = true;
        setActing(true);
        try {
            if (await (action === 'archive' ? onArchive() : handoff())) setConfirmation((previous) => ({...previous, open: false}));
        } finally {
            actingRef.current = false;
            setActing(false);
        }
    };

    const blocks = useMemo(() => buildTranscript(messages), [messages]);
    const liveTasks = session.background_tasks ?? [];
    const tasks = useMemo(() => backgroundTasks(messages, session.background_tasks ?? []), [messages, session.background_tasks]);
    const [tasksAnchor, setTasksAnchor] = useState<HTMLElement | null>(null);
    // Archiving or handing off ends the process, and every background task
    // it runs: ask first while any is running.
    const [confirmation, setConfirmation] = useState<{action: 'archive' | 'handoff'; open: boolean}>({action: 'archive', open: false});
    const guarded = (action: 'archive' | 'handoff') => () => {
        setMenuAnchor(null);
        if (action === 'archive' || liveTasks.length > 0) setConfirmation({action, open: true});
        else void runAction(action);
    };
    const turnInFlight = isBusyStatus(session.status);
    const pendingId = pendingRequestId(blocks, turnInFlight);
    const isClosed = session.status === 'closed';

    // reveal scrolls the conversation to the call that started a task and
    // flashes it, so "where did this come from" is one click from the panel.
    const flash = (selector: string) => {
        const el = scrollRef.current?.querySelector<HTMLElement>(selector);
        if (!el) return;
        revealElement(el);
        el.animate?.([{outline: '2px solid transparent'}, {outline: '2px solid var(--mui-palette-primary-main, #1976d2)'}, {outline: '2px solid transparent'}], {duration: 1600});
    };
    const reveal = (callId: string) => {
        setTasksAnchor(null);
        if (view !== 'chat') {
            openInChat({call: callId});
            return;
        }
        flash(anchorSelector({call: callId}));
    };
    useEffect(() => {
        if (!target || view !== 'chat') return;
        // After the chat has rendered (and laid out) in place of the list.
        const frame = requestAnimationFrame(() => {
            flash(anchorSelector(target));
            setTarget(null);
        });
        return () => cancelAnimationFrame(frame);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [target, view]);

    return (
        <Box sx={{display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0}}>
            <Stack
                direction="row"
                spacing={1}
                sx={{alignItems: 'center', px: 2, py: 1.25, borderBottom: 1, borderColor: 'divider', minWidth: 0, flexShrink: 0, '& .MuiIconButton-root': {minWidth: {xs: 40, md: 28}, minHeight: {xs: 40, md: 28}}}}
            >
                {onBack && (
                    <IconButton size="small" onClick={onBack} aria-label={t('common.back', {defaultValue: 'Back'})}>
                        <ArrowBack fontSize="small"/>
                    </IconButton>
                )}
                <Typography variant="subtitle1" noWrap sx={{fontWeight: 600, minWidth: 0, flex: {xs: 1, md: 'initial'}, color: 'text.primary'}}>{sessionTitle(session)}</Typography>
                <Box sx={{display: {xs: 'none', md: 'block'}}}><FolderChip path={session.project}/></Box>
                {session.status === 'failed' && <Chip size="small" color="error" variant="outlined" label={t('desk.statusFailed', {defaultValue: 'failed'})} sx={{color: (theme) => getReadableAccent(theme, 'error'), borderColor: (theme) => getReadableAccent(theme, 'error')}}/>}
                {isClosed && <Chip size="small" variant="outlined" label={t('desk.statusArchived', {defaultValue: 'archived'})}/>}
                <Box sx={{flex: 1, display: {xs: 'none', md: 'block'}}}/>
                <ToggleButtonGroup
                    size="small"
                    exclusive
                    value={view}
                    onChange={(_, next: SessionViewMode | null) => next && setView(next)}
                    aria-label={t('desk.viewSwitch', {defaultValue: 'View'})}
                    sx={{display: {xs: 'none', md: 'inline-flex'}, flexShrink: 0, '& .MuiToggleButton-root': {py: 0.25, px: 1.25, textTransform: 'none', fontSize: '0.8125rem', lineHeight: 1.5}}}
                >
                    <ToggleButton value="chat">{t('desk.viewChat', {defaultValue: 'Chat'})}</ToggleButton>
                    <ToggleButton value="trajectory">{t('desk.viewTrajectory', {defaultValue: 'Trajectory'})}</ToggleButton>
                </ToggleButtonGroup>
                {/* Always there, so it can be found before it is needed; while
                    work runs it names itself instead of hiding in a badge. */}
                {liveTasks.length > 0 ? (
                    <Button
                        size="small"
                        variant="outlined"
                        color="inherit"
                        startIcon={<Stream sx={{fontSize: '16px !important'}}/>}
                        onClick={(e) => setTasksAnchor(e.currentTarget)}
                        aria-label={t('desk.backgroundTasks', {defaultValue: 'Background tasks'})}
                        sx={{borderRadius: 4, borderColor: 'divider', color: 'text.secondary', py: 0, px: 1.25, minHeight: {xs: 40, md: 24}, minWidth: 0, textTransform: 'none', whiteSpace: 'nowrap', flexShrink: 0}}
                    >
                        {t('desk.backgroundRunningShort', {defaultValue: '{{count}} running', count: liveTasks.length})}
                    </Button>
                ) : (
                    <Tooltip title={t('desk.backgroundTasks', {defaultValue: 'Background tasks'})}>
                        <IconButton size="small" onClick={(e) => setTasksAnchor(e.currentTarget)} aria-label={t('desk.backgroundTasks', {defaultValue: 'Background tasks'})}>
                            <Stream fontSize="small"/>
                        </IconButton>
                    </Tooltip>
                )}
                <Tooltip title={expandAll
                    ? t('desk.collapseTools', {defaultValue: 'Collapse tool calls'})
                    : t('desk.expandTools', {defaultValue: 'Expand all tool calls'})}
                >
                    <IconButton sx={{display: {xs: 'none', md: view === 'chat' ? 'inline-flex' : 'none'}}} size="small" onClick={toggleExpand} aria-label={t('desk.expandTools', {defaultValue: 'Expand all tool calls'})} aria-pressed={expandAll}>
                        {expandAll ? <FoldUp fontSize="small"/> : <UnfoldMore fontSize="small"/>}
                    </IconButton>
                </Tooltip>
                {!isClosed && (
                    <Tooltip title={turnInFlight
                        ? t('desk.handoffBusy', {defaultValue: 'Stop or wait for the current turn to continue in a terminal'})
                        : t('desk.handoffHint', {defaultValue: 'Continue in terminal — copies a command that resumes this session through tingly-box'})}
                    >
                        {/* span: a disabled button fires no events for the tooltip */}
                        <Box component="span" sx={{display: {xs: 'none', md: 'inline-flex'}}}>
                            <IconButton size="small" disabled={turnInFlight || acting || queueSending || changingSettings} onClick={guarded('handoff')} aria-label={t('desk.handoff', {defaultValue: 'Continue in terminal'})}>
                                <Terminal fontSize="small"/>
                            </IconButton>
                        </Box>
                    </Tooltip>
                )}
                {!isClosed && (
                    <Tooltip title={t('desk.archiveHint', {defaultValue: 'Archive — ends the session; the folder and history stay'})}>
                        <IconButton sx={{display: {xs: 'none', md: 'inline-flex'}}} size="small" disabled={acting || queueSending || changingSettings} onClick={guarded('archive')} aria-label={t('desk.archive', {defaultValue: 'Archive'})}><Archive fontSize="small"/></IconButton>
                    </Tooltip>
                )}
                <IconButton sx={{display: {xs: 'inline-flex', md: 'none'}}} size="small" onClick={(event) => setMenuAnchor(event.currentTarget)} aria-label={t('desk.moreActions', {defaultValue: 'More actions'})}><MoreVert fontSize="small"/></IconButton>
            </Stack>
            <Menu anchorEl={menuAnchor} open={Boolean(menuAnchor)} onClose={() => setMenuAnchor(null)}>
                {/* Narrow screens: the title keeps the header, the switch lives here. */}
                <MenuItem onClick={() => {setView(view === 'chat' ? 'trajectory' : 'chat'); setMenuAnchor(null);}}>
                    {view === 'chat' ? t('desk.showTrajectory', {defaultValue: 'Show trajectory'}) : t('desk.showChat', {defaultValue: 'Show chat'})}
                </MenuItem>
                {view === 'chat' && <MenuItem onClick={() => {toggleExpand(); setMenuAnchor(null);}}>{expandAll ? t('desk.collapseTools', {defaultValue: 'Collapse tool calls'}) : t('desk.expandTools', {defaultValue: 'Expand all tool calls'})}</MenuItem>}
                {!isClosed && <MenuItem disabled={turnInFlight || acting || queueSending || changingSettings} onClick={guarded('handoff')}>{t('desk.handoff', {defaultValue: 'Continue in terminal'})}</MenuItem>}
                {!isClosed && <MenuItem disabled={acting || queueSending || changingSettings} onClick={guarded('archive')}>{t('desk.archive', {defaultValue: 'Archive'})}</MenuItem>}
            </Menu>
            <Popover
                open={tasksAnchor !== null}
                anchorEl={tasksAnchor}
                onClose={() => setTasksAnchor(null)}
                anchorOrigin={{vertical: 'bottom', horizontal: 'right'}}
                transformOrigin={{vertical: 'top', horizontal: 'right'}}
            >
                <BackgroundTasksPanel sessionId={session.id} tasks={tasks} onChanged={onRefresh} onReveal={reveal}/>
            </Popover>
            <ConfirmDialog
                open={confirmation.open}
                title={confirmation.action === 'archive'
                    ? t('desk.archiveConfirm', {defaultValue: 'Archive this session?'})
                    : t('desk.handoffWithTasks', {defaultValue: 'Continue in terminal and stop background tasks?'})}
                description={confirmation.action === 'archive'
                    ? t('desk.archiveDescription', {defaultValue: 'The current turn and background tasks will stop. History and unsent text stay available; this session cannot accept new messages.'})
                    : t('desk.tasksWillStop', {defaultValue: "{{count}} background task(s) still running will stop with this session's Claude process.", count: liveTasks.length})}
                loading={acting}
                cancelLabel={t('common.cancel', {defaultValue: 'Cancel'})}
                confirmingLabel={t('desk.working', {defaultValue: 'Working…'})}
                confirmLabel={confirmation.action === 'archive' ? t('desk.archive', {defaultValue: 'Archive'}) : t('desk.handoff', {defaultValue: 'Continue in terminal'})}
                confirmColor="warning"
                onClose={() => setConfirmation((previous) => ({...previous, open: false}))}
                onConfirm={() => void runAction(confirmation.action)}
            />

            <Box
                ref={scrollRef}
                role="region"
                aria-label={t('desk.conversation', {defaultValue: 'Conversation'})}
                onScroll={onScroll}
                sx={{flex: 1, minHeight: 0, overflowY: 'auto', overflowX: 'hidden', overscrollBehavior: 'contain', px: 2}}
            >
                <Box ref={contentRef} sx={{maxWidth: COLUMN_MAX_WIDTH, mx: 'auto', py: 3}}>
                    {messagesLoading && <LinearProgress aria-label={t('desk.loadingConversation', {defaultValue: 'Loading conversation'})} sx={{mb: 2}}/>}
                    {view === 'chat'
                        ? <Transcript blocks={blocks} pendingRequestId={pendingId} working={turnInFlight} expandAll={expandAll} onRespond={onRespond}/>
                        : <Trajectory blocks={blocks} working={turnInFlight} pendingRequestId={pendingId} project={session.project} onOpen={openInChat}/>}
                </Box>
            </Box>

            <Box sx={{px: {xs: 1, md: 2}, pb: {xs: 1, md: 2}, pt: 1, flexShrink: 0, maxHeight: '60%', overflowY: 'auto'}}>
                <Box sx={{maxWidth: COLUMN_MAX_WIDTH, mx: 'auto'}}>
                    {(away || pendingId) && <Stack direction="row" spacing={1} sx={{mb: 0.5, justifyContent: 'flex-end'}}>
                        {pendingId && <Button size="small" color="warning" onClick={() => {
                            if (view !== 'chat') {
                                openInChat({request: pendingId});
                                return;
                            }
                            const card = scrollRef.current?.querySelector<HTMLElement>(`[data-request-id="${CSS.escape(pendingId)}"]`);
                            if (card) {revealElement(card); card.querySelector<HTMLTextAreaElement>('textarea')?.focus({preventScroll: true});}
                        }}>{t('desk.reviewRequest', {defaultValue: 'Review pending request'})}</Button>}
                        {away && <Button size="small" startIcon={<KeyboardArrowDown/>} onClick={latest}>{unread ? t('desk.newActivity', {defaultValue: 'New activity — latest'}) : t('desk.backToLatest', {defaultValue: 'Back to latest'})}</Button>}
                    </Stack>}
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
                                        flex: 1, minWidth: 0, overflowX: 'auto', whiteSpace: 'nowrap', fontFamily: fontMono, fontSize: fontSizes.md,
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
                    {queued.length > 0 && (
                        <Stack spacing={0.5} sx={{mb: 1, maxHeight: 160, overflowY: 'auto'}}>
                            <Stack direction="row" sx={{alignItems: 'center'}}>
                                <Typography variant="caption" sx={{color: 'text.secondary', flex: 1}}>
                                    {queueSending
                                        ? t('desk.sendingQueue', {defaultValue: 'Sending queued messages…'})
                                        : queueHeld === 'paused'
                                            ? t('desk.queuePaused', {defaultValue: 'Unsent messages — automatic sending paused'})
                                        : queueHeld === 'restored'
                                        ? t('desk.queueRestored', {defaultValue: 'Recovered unsent messages — review the conversation before sending again.'})
                                        : queueHeld || isClosed
                                            ? t('desk.queuedHeld', {defaultValue: 'Not sent — the last turn or send did not complete'})
                                        : turnInFlight
                                        ? t('desk.queuedHint', {defaultValue: 'Queued — sent when the current turn ends'})
                                        : t('desk.queuedHeld', {defaultValue: 'Not sent — the last turn or send did not complete'})}
                                </Typography>
                                {!turnInFlight && !isClosed && (
                                    <Button size="small" disabled={queueSending || changingSettings || acting} onClick={onSendQueuedNow}>{t('desk.sendNow', {defaultValue: 'Send now'})}</Button>
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
                                        <IconButton size="small" disabled={queueSending} sx={{mt: -0.25, minWidth: {xs: 40, md: 28}, minHeight: {xs: 40, md: 28}}} onClick={() => onUnqueue(i)} aria-label={t('desk.unqueue', {defaultValue: 'Take back into the input'})}>
                                            <Close sx={{fontSize: 16}}/>
                                        </IconButton>
                                    </Tooltip>
                                </Stack>
                            ))}
                        </Stack>
                    )}
                    {isClosed && draft && <Alert severity="info" variant="outlined" sx={{mb: 1, '& .MuiAlert-message': {minWidth: 0, flex: 1}}} action={<Button size="small" onClick={() => copy(draft)}>{copied ? t('common.copied', {defaultValue: 'Copied'}) : t('common.copy', {defaultValue: 'Copy'})}</Button>}>
                        <Typography variant="caption">{t('desk.unsentDraft', {defaultValue: 'Unsent draft'})}</Typography>
                        <Typography variant="body2" sx={{whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', maxHeight: 120, overflowY: 'auto'}}>{draft}</Typography>
                    </Alert>}
                    {isClosed ? (
                        <Alert severity="info" variant="outlined">
                            {t('desk.archived', {defaultValue: 'This session is archived — the folder and history stay, but it can no longer be steered.'})}
                        </Alert>
                    ) : (
                        <Composer
                            key={session.id}
                            text={draft}
                            onTextChange={onDraftChange}
                            onAccepted={onDraftAccepted}
                            onStop={turnInFlight ? onInterrupt : undefined}
                            placeholder={turnInFlight
                                ? (pendingId
                                    ? t('desk.waitingForYou', {defaultValue: 'Waiting for your answer above…'})
                                    : t('desk.queuePlaceholder', {defaultValue: 'Queue a follow-up…'}))
                                : t('desk.messagePlaceholder', {defaultValue: 'Reply…'})}
                            canSubmit={!changingSettings && !acting}
                            onSubmit={onSend}
                            minRows={1}
                            context={(
                                <>
                                    <ProfileSelect disabled={changingSettings || acting || queueSending} value={session.profile} onChange={(p) => void changeSetting(() => onProfileChange(p))}/>
                                    <ModelSelect disabled={changingSettings || acting || queueSending} profile={session.profile} value={session.model} onChange={(m) => void changeSetting(() => onModelChange(m))}/>
                                    <PermissionModeSelect
                                        disabled={changingSettings || acting || queueSending}
                                        value={session.permission_mode}
                                        permissionModes={permissionModes}
                                        onChange={(m) => void changeSetting(() => onPermissionModeChange(m))}
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
