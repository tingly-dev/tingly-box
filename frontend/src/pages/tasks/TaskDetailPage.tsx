// One task: the conversation on the left, the artifact (branch, changes,
// actions) on the right. On a phone the two become tabs and the composer
// sticks to the bottom, because the two things a person does from a phone
// are answer an approval and send one more instruction.
import {useCallback, useMemo, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {useNavigate, useParams} from 'react-router-dom';
import {
    Alert, Box, Button, Card, Chip, CircularProgress, Divider, IconButton, ListItemText, Menu, MenuItem, Stack, Tab, Tabs,
    TextField, Tooltip, Typography, useMediaQuery, useTheme,
} from '@mui/material';
import {PageLayout} from '@/components/PageLayout';
import ConfirmDialog from '@/components/ConfirmDialog';
import EmptyState from '@/components/EmptyState';
import {ArrowBack, Block as IconStop, Delete as IconArchive, Send as IconSend} from '@/components/icons';
import {useNotify} from '@/hooks/useNotify';
import {agentApi, isActiveStatus, type PermissionMode} from '@/services/agentApi';
import EventTimeline, {type PendingRequest} from './EventTimeline';
import ChangesPanel from './ChangesPanel';
import {PERMISSION_MODES, permissionModeKey, relativeTime, shortId, StatusChip, useSessionPoll} from './taskShared';

const TaskDetailPage = () => {
    const {sessionId} = useParams<{sessionId: string}>();
    const {t} = useTranslation();
    const navigate = useNavigate();
    const notify = useNotify();
    const theme = useTheme();
    const isPhone = useMediaQuery(theme.breakpoints.down('md'));

    const {session, workspace, events, loading, notFound, refresh, setSession} = useSessionPoll(sessionId);
    const [tab, setTab] = useState<'conversation' | 'changes'>('conversation');
    const [text, setText] = useState('');
    const [sending, setSending] = useState(false);
    const [archiveOpen, setArchiveOpen] = useState(false);
    const [archiving, setArchiving] = useState(false);
    const [modeAnchor, setModeAnchor] = useState<HTMLElement | null>(null);

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

    // A request is pending while the session waits and no response event
    // has followed it in the log.
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
                <EmptyState
                    title={t('tasks.detail.notFound')}
                    primaryAction={{label: t('tasks.detail.back'), onClick: () => navigate('/tasks')}}
                />
            </PageLayout>
        );
    }

    const active = isActiveStatus(session?.status);
    // A failed turn is retried by changing what caused it (often the
    // permission mode) and sending again, as long as the checkout exists.
    const retryable = session?.status === 'failed' && workspace?.state === 'ready';
    const canSteer = (active && session?.status !== 'waiting_input') || retryable;
    const hint = session?.status === 'waiting_input'
        ? t('tasks.detail.waiting')
        : session?.status === 'running' || session?.status === 'queued'
            ? t('tasks.detail.running')
            : session?.status === 'idle'
                ? t('tasks.detail.idle')
                : undefined;

    const header = session && (
        <Stack spacing={1}>
            <Stack direction="row" spacing={1} sx={{alignItems: 'center', minWidth: 0}}>
                <IconButton size="small" onClick={() => navigate('/tasks')} aria-label={t('tasks.detail.back')}>
                    <ArrowBack fontSize="small" />
                </IconButton>
                <Typography variant="h6" sx={{flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap'}}>
                    {session.title || session.prompt}
                </Typography>
                <StatusChip status={session.status} />
            </Stack>
            <Stack direction="row" spacing={1} sx={{alignItems: 'center', flexWrap: 'wrap', rowGap: 0.5, pl: {xs: 0, sm: 5}}}>
                <Typography variant="caption" color="text.secondary">{relativeTime(session.last_active_at)}</Typography>
                {/* The mode is a concrete value, editable in place while the
                    session is active; a change takes effect from the next turn. */}
                <Tooltip title={t(`tasks.mode.${permissionModeKey(session.permission_mode)}Help`)}>
                    <Chip
                        size="small"
                        variant="outlined"
                        label={`${t('tasks.mode.label')}: ${t(`tasks.mode.${permissionModeKey(session.permission_mode)}`)}`}
                        onClick={active || retryable ? (e) => setModeAnchor(e.currentTarget) : undefined}
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
                {session.usage && (session.usage.input_tokens > 0 || session.usage.output_tokens > 0) && (
                    <Typography variant="caption" color="text.secondary">
                        · {t('tasks.detail.tokens', {input: session.usage.input_tokens, output: session.usage.output_tokens})}
                        {session.usage.cost > 0 && ` · ${t('tasks.detail.cost', {cost: session.usage.cost.toFixed(3)})}`}
                    </Typography>
                )}
                <Box sx={{flex: 1}} />
                {(session.status === 'running' || session.status === 'waiting_input') && (
                    <Tooltip title={t('tasks.detail.interrupt')}>
                        <IconButton size="small" onClick={interrupt}><IconStop fontSize="small" /></IconButton>
                    </Tooltip>
                )}
                {session.status !== 'archived' && (
                    <Tooltip title={t('tasks.detail.archive')}>
                        <IconButton size="small" onClick={() => setArchiveOpen(true)}><IconArchive fontSize="small" /></IconButton>
                    </Tooltip>
                )}
            </Stack>
        </Stack>
    );

    const composer = session && session.status !== 'archived' && (active || retryable) && (
        <Stack direction="row" spacing={1} sx={{alignItems: 'flex-end'}}>
            <TextField
                fullWidth
                size="small"
                multiline
                maxRows={5}
                placeholder={t('tasks.detail.steerPlaceholder')}
                value={text}
                onChange={(e) => setText(e.target.value)}
                disabled={!active && !retryable}
                onKeyDown={(e) => {
                    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter' && text.trim()) send();
                }}
            />
            <Button
                variant="contained"
                onClick={send}
                disabled={!canSteer || !text.trim() || sending}
                startIcon={sending ? <CircularProgress size={16} color="inherit" /> : <IconSend />}
                sx={{whiteSpace: 'nowrap'}}
            >
                {t('tasks.detail.send')}
            </Button>
        </Stack>
    );

    const conversation = session && (
        <Stack spacing={1.5}>
            {session.status === 'failed' && (
                <Alert severity="error">
                    {t('tasks.detail.failed')}{session.error ? `: ${session.error}` : ''}
                    {retryable && <Typography variant="body2" sx={{mt: 0.5}}>{t('tasks.detail.retryHint')}</Typography>}
                </Alert>
            )}
            <EventTimeline events={events} pending={pending} onRespond={respond} />
            {hint && (
                <Typography variant="caption" color="text.secondary" sx={{display: 'flex', alignItems: 'center', gap: 1}}>
                    {(session.status === 'running' || session.status === 'queued') && <CircularProgress size={12} />}
                    {hint}
                </Typography>
            )}
        </Stack>
    );

    const changes = session && (
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
                <Typography variant="overline" color="text.secondary">{t('tasks.detail.checkout')}</Typography>
                <Typography variant="caption" sx={{fontFamily: 'monospace', wordBreak: 'break-all'}}>{workspace?.path ?? '—'}</Typography>
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
                <Stack spacing={2} sx={{height: '100%'}}>
                    {header}
                    {isPhone ? (
                        <>
                            <Tabs value={tab} onChange={(_, v) => setTab(v)} variant="fullWidth">
                                <Tab value="conversation" label={t('tasks.detail.conversation')} />
                                <Tab value="changes" label={`${t('tasks.detail.changes')}${(session.artifact?.changed_files ?? 0) > 0 ? ` (${session.artifact.changed_files})` : ''}`} />
                            </Tabs>
                            <Box sx={{pb: composer ? 10 : 0}}>
                                {tab === 'conversation' ? conversation : changes}
                            </Box>
                            {composer && tab === 'conversation' && (
                                <Box sx={{position: 'fixed', left: 0, right: 0, bottom: 0, p: 1.5, bgcolor: 'background.paper', borderTop: '1px solid', borderColor: 'divider', zIndex: 2}}>
                                    {composer}
                                </Box>
                            )}
                        </>
                    ) : (
                        <Box sx={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) minmax(320px, 400px)', gap: 3, alignItems: 'start'}}>
                            <Stack spacing={2}>
                                <Card variant="outlined" sx={{p: 2}}>{conversation}</Card>
                                {composer}
                            </Stack>
                            <Card variant="outlined" sx={{p: 2, position: 'sticky', top: 16}}>{changes}</Card>
                        </Box>
                    )}
                </Stack>
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
