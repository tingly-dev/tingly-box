// One task, laid out like a chat and built like one: the page is a fixed
// frame the height of the viewport — top bar at the top, composer at the
// bottom — and only the conversation between them scrolls. Nothing floats
// or follows the content. The changes live in a side panel with its own
// scroll (open by itself on desktop once there are changes); on a phone
// it slides in as a drawer, so the two things a person does from a phone —
// answer a question, send one more instruction — never leave the screen.
import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {useNavigate, useParams} from 'react-router-dom';
import {
    Alert, Badge, Box, Chip, CircularProgress, Divider, Drawer, IconButton, InputBase, ListItemText, Menu, MenuItem,
    Paper, Stack, Tooltip, Typography, useMediaQuery, useTheme,
} from '@mui/material';
import {PageLayout} from '@/components/PageLayout';
import ConfirmDialog from '@/components/ConfirmDialog';
import EmptyState from '@/components/EmptyState';
import {
    Archive as IconArchive, ArrowBack, Block as IconStop, Close as IconClose, CompareArrows as IconChanges,
    FolderOpen as IconFolder, KeyboardArrowDown as IconCaret, Send as IconSend, Shield as IconShield,
} from '@/components/icons';
import {useNotify} from '@/hooks/useNotify';
import {agentApi, isActiveStatus, type PermissionMode} from '@/services/agentApi';
import EventTimeline, {type PendingRequest} from './EventTimeline';
import ChangesPanel from './ChangesPanel';
import {PERMISSION_MODES, permissionModeKey, relativeTime, shortId, StatusChip, useSessionPoll} from './taskShared';

const COLUMN = 860;
const PANEL = 400;
// The layout's scroll container is 100vh tall with its own padding
// (mobileContentSx: 72px top on phones for the app bar, 24px otherwise,
// 24px bottom); the frame fills exactly what is left so nothing outside
// it ever scrolls.
const FRAME_HEIGHT = {xs: 'calc(100vh - 96px)', md: 'calc(100vh - 48px)'};

const TaskDetailPage = () => {
    const {sessionId} = useParams<{sessionId: string}>();
    const {t} = useTranslation();
    const navigate = useNavigate();
    const notify = useNotify();
    const theme = useTheme();
    const isPhone = useMediaQuery(theme.breakpoints.down('md'));

    const {session, workspace, events, loading, notFound, refresh, setSession} = useSessionPoll(sessionId);
    const [text, setText] = useState('');
    const [sending, setSending] = useState(false);
    const [archiveOpen, setArchiveOpen] = useState(false);
    const [archiving, setArchiving] = useState(false);
    const [modeAnchor, setModeAnchor] = useState<HTMLElement | null>(null);
    const [panelChoice, setPanelChoice] = useState<boolean>();
    const bottomRef = useRef<HTMLDivElement>(null);
    const lastCount = useRef(0);

    const changed = session?.artifact?.changed_files ?? 0;
    // Desktop: open by itself once there is something to look at; the user's
    // own toggle wins after that. Phone: closed until asked.
    const panelOpen = panelChoice ?? (!isPhone && changed > 0);

    // Follow the conversation as it grows, the way a chat does.
    useEffect(() => {
        if (events.length > lastCount.current) {
            lastCount.current = events.length;
            bottomRef.current?.scrollIntoView({block: 'end', behavior: 'smooth'});
        }
    }, [events.length]);

    const changeMode = async (mode: PermissionMode) => {
        setModeAnchor(null);
        if (!sessionId || !session || mode === (session.permission_mode ?? '')) return;
        const res = await agentApi.setPermissionMode(sessionId, mode);
        if (!res.ok) {
            notify.error(res.error);
            return;
        }
        setSession(res.data.session);
        await refresh();
    };

    const pending = useMemo<PendingRequest[]>(() => {
        if (session?.status !== 'waiting_input') return [];
        const answered = new Set(events.filter((e) => e.kind === 'approval_response' || e.kind === 'ask_response').map((e) => e.request_id));
        return events
            .filter((e) => (e.kind === 'approval_request' || e.kind === 'ask_request') && e.request_id && !answered.has(e.request_id))
            .map((e) => ({requestId: e.request_id as string, kind: e.kind === 'ask_request' ? 'ask' : 'approval'}));
    }, [events, session?.status]);

    const respond = useCallback(async (requestId: string, approved: boolean, answer?: string) => {
        if (!sessionId) return;
        const res = await agentApi.respond(sessionId, {request_id: requestId, approved, answer});
        if (!res.ok) notify.error(res.error);
        await refresh();
    }, [sessionId, notify, refresh]);

    const send = async () => {
        if (!sessionId || !text.trim()) return;
        setSending(true);
        const res = await agentApi.sendMessage(sessionId, text.trim());
        setSending(false);
        if (!res.ok) {
            notify.error(res.error);
            return;
        }
        setText('');
        await refresh();
    };

    const interrupt = async () => {
        if (!sessionId) return;
        const res = await agentApi.interrupt(sessionId);
        if (!res.ok) notify.error(res.error);
    };

    const archive = async () => {
        if (!sessionId) return;
        setArchiving(true);
        const res = await agentApi.archive(sessionId);
        setArchiving(false);
        setArchiveOpen(false);
        if (!res.ok) {
            notify.error(res.error);
            return;
        }
        setSession(res.data.session);
    };

    if (notFound) {
        return (
            <PageLayout loading={false}>
                <EmptyState title={t('tasks.detail.notFound')} primaryAction={{label: t('tasks.detail.back'), onClick: () => navigate('/tasks')}} />
            </PageLayout>
        );
    }

    const active = isActiveStatus(session?.status);
    const working = session?.status === 'running' || session?.status === 'queued';
    const retryable = session?.status === 'failed' && workspace?.state === 'ready';
    const canSteer = (active && session?.status !== 'waiting_input') || retryable;
    const showComposer = !!session && session.status !== 'archived' && (active || retryable);
    const folderName = workspace?.path ? workspace.path.split(/[\\/]/).filter(Boolean).pop() : undefined;
    const modeKey = permissionModeKey(session?.permission_mode);

    const topBar = session && (
        <Stack
            direction="row"
            spacing={1}
            sx={{alignItems: 'center', minWidth: 0, flexShrink: 0, pb: 1, borderBottom: '1px solid', borderColor: 'divider'}}
        >
            <IconButton size="small" onClick={() => navigate('/tasks')} aria-label={t('tasks.detail.back')}>
                <ArrowBack fontSize="small" />
            </IconButton>
            <Stack sx={{flex: 1, minWidth: 0}}>
                <Typography variant="subtitle1" sx={{fontWeight: 600, lineHeight: 1.3, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap'}}>
                    {session.title || session.prompt}
                </Typography>
                <Stack direction="row" spacing={1} sx={{alignItems: 'center', minWidth: 0}}>
                    {folderName && (
                        <Tooltip title={workspace?.path ?? ''}>
                            <Typography variant="caption" color="text.secondary" sx={{display: 'inline-flex', alignItems: 'center', gap: 0.5, minWidth: 0}}>
                                <IconFolder sx={{fontSize: 14}} />
                                <Box component="span" sx={{overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap'}}>{folderName}</Box>
                            </Typography>
                        </Tooltip>
                    )}
                    <Typography variant="caption" color="text.disabled" sx={{whiteSpace: 'nowrap'}}>{relativeTime(session.last_active_at)}</Typography>
                    {!isPhone && session.usage && (session.usage.input_tokens > 0 || session.usage.output_tokens > 0) && (
                        <Typography variant="caption" color="text.disabled" sx={{whiteSpace: 'nowrap'}}>
                            · {t('tasks.detail.tokens', {input: session.usage.input_tokens, output: session.usage.output_tokens})}
                            {session.usage.cost > 0 && ` · ${t('tasks.detail.cost', {cost: session.usage.cost.toFixed(3)})}`}
                        </Typography>
                    )}
                </Stack>
            </Stack>
            <StatusChip status={session.status} />
            <Tooltip title={t('tasks.detail.changes')}>
                <IconButton size="small" color={panelOpen ? 'primary' : 'default'} onClick={() => setPanelChoice(!panelOpen)} aria-label={t('tasks.detail.changes')}>
                    <Badge badgeContent={changed || undefined} color="primary" max={99}>
                        <IconChanges fontSize="small" />
                    </Badge>
                </IconButton>
            </Tooltip>
            {(session.status === 'running' || session.status === 'waiting_input') && (
                <Tooltip title={t('tasks.detail.interrupt')}>
                    <IconButton size="small" onClick={interrupt} aria-label={t('tasks.detail.interrupt')}><IconStop fontSize="small" /></IconButton>
                </Tooltip>
            )}
            {session.status !== 'archived' && (
                <Tooltip title={t('tasks.detail.archive')}>
                    <IconButton size="small" onClick={() => setArchiveOpen(true)} aria-label={t('tasks.detail.archive')}><IconArchive fontSize="small" /></IconButton>
                </Tooltip>
            )}
        </Stack>
    );

    const composer = showComposer && session && (
        <Box sx={{flexShrink: 0, pt: 1.5, width: '100%', maxWidth: COLUMN, mx: 'auto'}}>
            <Paper
                variant="outlined"
                sx={{borderRadius: 3, px: 1.5, pt: 1.25, pb: 1, boxShadow: (th) => th.shadows[1], '&:focus-within': {borderColor: 'primary.main'}}}
            >
                <InputBase
                    fullWidth
                    multiline
                    maxRows={6}
                    placeholder={session.status === 'waiting_input' ? t('tasks.detail.waitingShort') : t('tasks.detail.steerPlaceholder')}
                    value={text}
                    onChange={(e) => setText(e.target.value)}
                    disabled={!canSteer}
                    onKeyDown={(e) => {
                        if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
                            e.preventDefault();
                            if (text.trim() && canSteer && !sending) send();
                        }
                    }}
                    sx={{px: 0.5, fontSize: 14, lineHeight: 1.6}}
                />
                <Stack direction="row" spacing={1} sx={{alignItems: 'center', pt: 0.75}}>
                    <Tooltip title={t(`tasks.mode.${modeKey}Help`)}>
                        <Chip
                            size="small"
                            variant="outlined"
                            icon={<IconShield />}
                            deleteIcon={<IconCaret />}
                            onDelete={(e) => setModeAnchor(e.currentTarget.parentElement)}
                            onClick={(e) => setModeAnchor(e.currentTarget)}
                            label={t(`tasks.mode.${modeKey}`)}
                        />
                    </Tooltip>
                    <Menu open={!!modeAnchor} anchorEl={modeAnchor} onClose={() => setModeAnchor(null)}>
                        {PERMISSION_MODES.map((m) => {
                            const key = permissionModeKey(m);
                            return (
                                <MenuItem key={key} selected={(session.permission_mode ?? '') === m} onClick={() => changeMode(m)}>
                                    <ListItemText primary={t(`tasks.mode.${key}`)} secondary={t(`tasks.mode.${key}Help`)} />
                                </MenuItem>
                            );
                        })}
                    </Menu>
                    <Typography variant="caption" color="text.disabled" sx={{flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', display: {xs: 'none', sm: 'block'}}}>
                        {working ? t('tasks.detail.working') : session.status === 'waiting_input' ? t('tasks.detail.waiting') : t('tasks.detail.enterHint')}
                    </Typography>
                    <Box sx={{flex: {xs: 1, sm: 0}}} />
                    <IconButton
                        color="primary"
                        size="small"
                        onClick={send}
                        disabled={!canSteer || !text.trim() || sending}
                        aria-label={t('tasks.detail.send')}
                        sx={{bgcolor: 'primary.main', color: 'primary.contrastText', '&:hover': {bgcolor: 'primary.dark'}, '&.Mui-disabled': {bgcolor: 'action.disabledBackground', color: 'action.disabled'}}}
                    >
                        {sending ? <CircularProgress size={16} color="inherit" /> : <IconSend fontSize="small" />}
                    </IconButton>
                </Stack>
            </Paper>
        </Box>
    );

    const conversation = session && (
        <Stack spacing={2} sx={{py: 2, maxWidth: COLUMN, mx: 'auto', width: '100%'}}>
            {session.status === 'failed' && (
                <Alert severity="error" variant="outlined">
                    {t('tasks.detail.failed')}{session.error ? `: ${session.error}` : ''}
                    {retryable && <Typography variant="body2" sx={{mt: 0.5}}>{t('tasks.detail.retryHint')}</Typography>}
                </Alert>
            )}
            <EventTimeline events={events} pending={pending} onRespond={respond} />
            {working && (
                <Stack direction="row" spacing={1} sx={{alignItems: 'center', pl: {xs: 0, sm: 4.5}}}>
                    <CircularProgress size={12} />
                    <Typography variant="caption" color="text.secondary">{t('tasks.detail.working')}</Typography>
                </Stack>
            )}
            <div ref={bottomRef} />
        </Stack>
    );

    const panel = session && (
        <Stack spacing={2}>
            <ChangesPanel
                session={session}
                workspace={workspace}
                onPushed={(s) => {
                    setSession(s);
                    notify.success(t('tasks.detail.pushDone', {branch: workspace?.branch ?? s.artifact?.branch ?? ''}));
                }}
                onError={(m) => notify.error(m)}
            />
            <Divider />
            <Stack spacing={0.5}>
                {workspace?.branch && (
                    <>
                        <Typography variant="overline" color="text.secondary">{t('tasks.detail.checkout')}</Typography>
                        <Typography variant="caption" sx={{fontFamily: 'monospace', wordBreak: 'break-all'}}>{workspace.path}</Typography>
                    </>
                )}
                {session.cc_session_id && (
                    <>
                        <Typography variant="overline" color="text.secondary">{t('tasks.detail.claudeSession')}</Typography>
                        <Typography variant="caption" sx={{fontFamily: 'monospace'}}>{shortId(session.cc_session_id)}</Typography>
                    </>
                )}
                <Typography variant="overline" color="text.secondary">{t('tasks.detail.createdBy')}</Typography>
                <Typography variant="caption">{session.created_by}</Typography>
            </Stack>
        </Stack>
    );

    return (
        <PageLayout loading={loading && !session}>
            {session && (
                <Box sx={{height: FRAME_HEIGHT, display: 'flex', flexDirection: 'column', minHeight: 0}}>
                    {topBar}
                    <Box sx={{display: 'flex', gap: 3, flex: 1, minHeight: 0, minWidth: 0}}>
                        {/* Conversation column: the scroll box, then the composer, fixed. */}
                        <Box sx={{flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', minHeight: 0}}>
                            <Box sx={{flex: 1, minHeight: 0, overflowY: 'auto', pr: 0.5}}>
                                {conversation}
                            </Box>
                            {composer}
                        </Box>
                        {!isPhone && panelOpen && (
                            <Box sx={{width: PANEL, flexShrink: 0, minHeight: 0, overflowY: 'auto', pt: 2}}>
                                <Paper variant="outlined" sx={{p: 2, borderRadius: 2}}>{panel}</Paper>
                            </Box>
                        )}
                    </Box>
                    {isPhone && (
                        <Drawer anchor="right" open={panelOpen} onClose={() => setPanelChoice(false)} slotProps={{paper: {sx: {width: 'min(100vw, 420px)', p: 2}}}}>
                            <Stack direction="row" sx={{alignItems: 'center', mb: 1}}>
                                <Typography variant="subtitle1" sx={{flex: 1, fontWeight: 600}}>{t('tasks.detail.changes')}</Typography>
                                <IconButton size="small" onClick={() => setPanelChoice(false)} aria-label={t('common.close')}><IconClose fontSize="small" /></IconButton>
                            </Stack>
                            {panel}
                        </Drawer>
                    )}
                </Box>
            )}
            <ConfirmDialog
                open={archiveOpen}
                title={t('tasks.detail.archiveConfirmTitle')}
                description={t('tasks.detail.archiveConfirm')}
                confirmLabel={t('tasks.detail.archive')}
                confirmColor="warning"
                loading={archiving}
                onClose={() => setArchiveOpen(false)}
                onConfirm={archive}
            />
        </PageLayout>
    );
};

export default TaskDetailPage;
