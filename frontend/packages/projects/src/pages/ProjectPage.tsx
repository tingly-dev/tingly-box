import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Alert, Badge, Box, Button, Drawer, IconButton, Skeleton, Tooltip, Typography, useMediaQuery, useTheme } from '@mui/material';
import { Link as RouterLink, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type Project, type ProjectMessage, type Thread, type ThreadEvent } from '../client';
import { ConversationPane } from '../components/project/ConversationPane';
import { OverviewPane } from '../components/project/OverviewPane';
import { ProjectSettingsDialog, type SettingsTab } from '../components/project/ProjectSettingsDialog';
import { PANE_DEFAULT, PaneResizer, clampPane } from '../components/project/PaneResizer';
import { ThreadPanel } from '../components/project/ThreadPanel';
import { OverviewPane as OverviewIcon, Pause, Settings, Star, StarBorder } from '../components/icons';
import { useNotify } from '../hooks/useNotify';
import { useProjectEvents, type ProjectEvent } from '../hooks/useProjectEvents';
import { useProjectsHost } from '../host/ProjectsHost';
import { readLocal, writeLocal } from '../utils/storage';


function upsert<T extends { id: string }>(list: T[], item: T): T[] {
    const i = list.findIndex((x) => x.id === item.id);
    if (i < 0) return [...list, item];
    const next = list.slice();
    next[i] = item;
    return next;
}

export default function ProjectPage() {
    const { id = '' } = useParams();
    const { t } = useTranslation();
    const theme = useTheme();
    const narrow = useMediaQuery(theme.breakpoints.down('lg'));
    const navigate = useNavigate();
    const notify = useNotify();
    const { onProjectsChanged: refresh, paths: routes } = useProjectsHost();
    const [params, setParams] = useSearchParams();
    const threadId = params.get('thread') ?? '';

    const [project, setProject] = useState<Project | null>(null);
    const [threads, setThreads] = useState<Thread[]>([]);
    const [messages, setMessages] = useState<ProjectMessage[] | null>(null);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [threadEvents, setThreadEvents] = useState<ThreadEvent[] | null>(null);
    const [paneOpen, setPaneOpen] = useState(() => readLocal('overviewOpen') !== '0');
    const [paneWidth, setPaneWidth] = useState(() => clampPane(Number(readLocal('paneWidth')) || PANE_DEFAULT));
    const row = useRef<HTMLDivElement>(null);
    // On narrow screens the pane is a drawer, closed until asked for.
    const [drawerOpen, setDrawerOpen] = useState(false);
    const [settingsTab, setSettingsTab] = useState<SettingsTab | null>(null);
    const [memoryKey, setMemoryKey] = useState(0);
    const [libraryKey, setLibraryKey] = useState(0);

    const load = useCallback(async () => {
        try {
            const [d, m] = await Promise.all([
                unwrap(api.GET('/api/v1/projects/{id}', { params: { path: { id } } })),
                unwrap(api.GET('/api/v1/projects/{id}/messages', { params: { path: { id }, query: { limit: 200 } } })),
            ]);
            setProject(d.project);
            setThreads(d.threads);
            setBusy(d.coordinator_busy);
            setMessages(m.messages);
            setError(null);
        } catch (e) {
            setError((e as Error).message);
        }
    }, [id]);

    const loadThread = useCallback(async (tid: string) => {
        try {
            const d = await unwrap(api.GET('/api/v1/projects/{id}/threads/{tid}', { params: { path: { id, tid } } }));
            setThreadEvents(d.events);
            setThreads((list) => upsert(list, d.thread));
        } catch (e) {
            notify((e as Error).message, 'error');
        }
    }, [id, notify]);

    useEffect(() => {
        setProject(null);
        setMessages(null);
        void load();
    }, [load]);

    // Keep the conversation usable when the window shrinks; the saved width
    // comes back once there is room again.
    useEffect(() => {
        const fit = () => setPaneWidth(clampPane(Number(readLocal('paneWidth')) || PANE_DEFAULT, row.current?.clientWidth));
        fit();
        window.addEventListener('resize', fit);
        return () => window.removeEventListener('resize', fit);
    }, []);

    useEffect(() => {
        setThreadEvents(null);
        if (threadId) void loadThread(threadId);
    }, [threadId, loadThread]);

    const onEvent = useCallback((ev: ProjectEvent) => {
        switch (ev.type) {
            case 'message':
                setMessages((list) => (list ? upsert(list, ev.data as ProjectMessage) : list));
                break;
            case 'message_delta': {
                const d = ev.data as { id: string; text: string };
                setMessages((list) => list?.map((m) => (m.id === d.id ? { ...m, content: m.content + d.text } : m)) ?? list);
                break;
            }
            case 'thread': {
                const th = ev.data as Thread;
                setThreads((list) => upsert(list, th));
                if (th.status !== 'working' && th.status !== 'waiting') setLibraryKey((k) => k + 1);
                refresh();
                break;
            }
            case 'thread_event': {
                const e = ev.data as ThreadEvent;
                if (e.thread_id === threadId) setThreadEvents((list) => (list ? upsert(list, e) : list));
                break;
            }
            case 'coordinator':
                setBusy((ev.data as { busy: boolean }).busy);
                break;
            case 'project': {
                const d = ev.data as Project & { memory?: string };
                if (d.memory !== undefined) setMemoryKey((k) => k + 1);
                else if (d.id) setProject(d);
                break;
            }
        }
    }, [threadId, refresh]);

    useProjectEvents(id, onEvent, () => {
        void load();
        if (threadId) void loadThread(threadId);
    });

    const selected = threads.find((th) => th.id === threadId);
    // A turn is in flight while the thread works or waits on a request it
    // raised (approvals only exist inside a running turn).
    const threadRunning = selected?.status === 'working'
        || (selected?.status === 'waiting' && !!threadEvents?.some((e) => (e.kind === 'approval' || e.kind === 'ask') && e.payload?.includes('"state":"pending"')));

    const threadMap = useMemo(() => new Map(threads.map((th) => [th.id, th])), [threads]);
    const waiting = threads.filter((th) => th.status === 'waiting').length;

    const openThread = (tid: string) => {
        setParams((p) => {
            p.set('thread', tid);
            return p;
        });
        if (narrow) setDrawerOpen(true);
        else if (!paneOpen) setPaneOpen(true);
    };
    const closeThread = () => setParams((p) => {
        p.delete('thread');
        return p;
    });
    const togglePane = () => {
        if (narrow) {
            setDrawerOpen((v) => !v);
            return;
        }
        setPaneOpen((v) => {
            writeLocal('overviewOpen', v ? '0' : '1');
            return !v;
        });
    };

    const patch = async (body: { starred?: boolean; paused?: boolean; archived?: boolean }) => {
        try {
            setProject(await unwrap(api.PATCH('/api/v1/projects/{id}', { params: { path: { id } }, body })));
            refresh();
        } catch (e) {
            notify((e as Error).message, 'error');
        }
    };

    if (error) {
        return (
            <Box sx={{ maxWidth: 720, mx: 'auto', pt: 8, px: 2 }}>
                <Alert severity="error" action={<Button color="inherit" component={RouterLink} to={routes.list}>{t('projects.all')}</Button>}>{error}</Alert>
            </Box>
        );
    }

    const blocked = !!project && (project.paused || !!project.archived_at);
    const banner = project?.archived_at ? (
        <Alert severity="info" sx={{ mb: 1 }} action={<Button color="inherit" size="small" onClick={() => void patch({ archived: false })}>{t('projectSettings.unarchive')}</Button>}>{t('project.archivedBanner')}</Alert>
    ) : project?.paused ? (
        <Alert severity="warning" icon={<Pause />} sx={{ mb: 1 }} action={<Button color="inherit" size="small" onClick={() => void patch({ paused: false })}>{t('projectSettings.resume')}</Button>}>{t('project.pausedBanner')}</Alert>
    ) : null;

    const pane = threadId ? (
        <ThreadPanel projectId={id} threadId={threadId} thread={selected} events={threadEvents} running={threadRunning} disabled={blocked} onBack={closeThread} />
    ) : (
        <OverviewPane projectId={id} threads={threads} onOpen={openThread} libraryKey={libraryKey} />
    );

    return (
        <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, px: { xs: 6, md: 3 }, py: 1, minHeight: 52, borderBottom: 1, borderColor: 'divider' }}>
                {project ? (
                    <Box sx={{ minWidth: 0, flex: 1 }}>
                        <Typography variant="h5" noWrap>{project.name}</Typography>
                        {project.goal && <Typography variant="caption" color="text.secondary" noWrap component="div">{project.goal}</Typography>}
                    </Box>
                ) : <Skeleton width={200} sx={{ flex: 1 }} />}
                {project && (
                    <>
                        <Tooltip title={project.starred ? t('common.unstar') : t('common.star')}>
                            <IconButton size="small" onClick={() => void patch({ starred: !project.starred })}>{project.starred ? <Star fontSize="small" color="primary" /> : <StarBorder fontSize="small" />}</IconButton>
                        </Tooltip>
                        <Tooltip title={t('project.overview')}>
                            <IconButton size="small" onClick={togglePane} aria-label={t('project.overview')}>
                                <Badge color="warning" variant="dot" invisible={waiting === 0}><OverviewIcon fontSize="small" /></Badge>
                            </IconButton>
                        </Tooltip>
                        <Tooltip title={t('projectSettings.title')}>
                            <IconButton size="small" onClick={() => setSettingsTab('general')} aria-label={t('projectSettings.title')}><Settings fontSize="small" /></IconButton>
                        </Tooltip>
                    </>
                )}
            </Box>

            <Box ref={row} sx={{ flex: 1, display: 'flex', minHeight: 0 }}>
                {messages === null ? (
                    <Box sx={{ flex: 1, p: 3 }}><Skeleton variant="rounded" height={80} /></Box>
                ) : (
                    <ConversationPane
                        messages={messages}
                        threads={threadMap}
                        busy={busy}
                        disabled={blocked || !project}
                        disabledReason={banner}
                        onOpenThread={openThread}
                        onSend={async (text) => {
                            try {
                                const m = await unwrap(api.POST('/api/v1/projects/{id}/messages', { params: { path: { id } }, body: { content: text } }));
                                setMessages((list) => (list ? upsert(list, m) : list));
                                setBusy(true);
                                return true;
                            } catch (e) {
                                notify((e as Error).message, 'error');
                                return false;
                            }
                        }}
                        onStartSuggestions={async (mid, indexes) => {
                            try {
                                const m = await unwrap(api.POST('/api/v1/projects/{id}/messages/{mid}/start', { params: { path: { id, mid } }, body: { indexes } }));
                                setMessages((list) => (list ? upsert(list, m) : list));
                            } catch (e) {
                                notify((e as Error).message, 'error');
                            }
                        }}
                    />
                )}
                {narrow ? (
                    <Drawer anchor="right" open={drawerOpen} onClose={() => setDrawerOpen(false)}
                        slotProps={{ paper: { sx: { width: { xs: '100%', sm: PANE_DEFAULT } } } }}>
                        {pane}
                    </Drawer>
                ) : paneOpen && (
                    <Box sx={{ position: 'relative', width: paneWidth, flexShrink: 0, borderLeft: 1, borderColor: 'divider', bgcolor: 'background.paper', minHeight: 0 }}>
                        <PaneResizer width={paneWidth} onWidth={setPaneWidth} onCommit={(w) => writeLocal('paneWidth', String(w))} />
                        {pane}
                    </Box>
                )}
            </Box>

            {project && settingsTab && (
                <ProjectSettingsDialog
                    project={project}
                    open
                    tab={settingsTab}
                    onTab={setSettingsTab}
                    onClose={() => setSettingsTab(null)}
                    onChanged={(p) => {
                        setProject(p);
                        if (!p.archived_at && project.archived_at) navigate(routes.project(p.id));
                    }}
                    memoryKey={memoryKey}
                />
            )}
        </Box>
    );
}
