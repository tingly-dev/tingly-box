import { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Box, Button, IconButton, Link, Skeleton, Tooltip, Typography } from '@mui/material';
import { Link as RouterLink, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type Project, type ProjectMessage, type Thread, type ThreadEvent } from '../client';
import { ConversationPane } from '../components/project/ConversationPane';
import { OverviewPane } from '../components/project/OverviewPane';
import { ProjectSettingsDialog, type SettingsTab } from '../components/project/ProjectSettingsDialog';
import { ThreadPanel } from '../components/project/ThreadPanel';
import { FloatingWindow } from '../components/workspace/FloatingWindow';
import { OverviewBand } from '../components/workspace/OverviewBand';
import { PlacementMenu } from '../components/workspace/PlacementMenu';
import { WorkspaceLayout, useWorkspacePane } from '../components/workspace/WorkspaceLayout';
import { FloatWindow, Pause, Settings, Star, StarBorder } from '../components/icons';
import { useNotify } from '../hooks/useNotify';
import { useProjectEvents, type ProjectEvent } from '../hooks/useProjectEvents';
import { usePlacement } from '../hooks/usePlacement';
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
    const { t } = useTranslation('projects');
    const navigate = useNavigate();
    const notify = useNotify();
    const { onProjectsChanged: refresh, paths: routes, embedded } = useProjectsHost();
    const [params, setParams] = useSearchParams();
    const threadId = params.get('thread') ?? '';

    const [project, setProject] = useState<Project | null>(null);
    const [threads, setThreads] = useState<Thread[]>([]);
    const [messages, setMessages] = useState<ProjectMessage[] | null>(null);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [threadEvents, setThreadEvents] = useState<ThreadEvent[] | null>(null);
    const pane = useWorkspacePane();
    const placement = usePlacement();
    // The Overview as a window or a band opens and closes with the header button; remembered per browser.
    const [windowOpen, setWindowOpen] = useState(() => readLocal('overviewWindowOpen') !== '0');
    const [bandOpen, setBandOpen] = useState(() => readLocal('overviewBandOpen') !== '0');
    const [libraryWindow, setLibraryWindow] = useState(false);
    // A narrow screen has one surface: the drawer. With the Overview on top the thread sits below it.
    const overviewAt = pane.narrow ? 'side' : placement.overview;
    // The thread picked in the side-pane Overview while "float from the Overview" is on. It floats over the
    // Overview; at most one window, since picking another thread only swaps it.
    const [overviewFloatId, setOverviewFloatId] = useState<string | null>(null);
    const floatedFromOverview = placement.overviewFloat && overviewAt === 'side' && !!threadId && overviewFloatId === threadId;
    const threadAt = pane.narrow || overviewAt === 'top' ? 'side' : floatedFromOverview ? 'float' : placement.thread;
    const remember = (key: string, set: (v: boolean) => void) => (v: boolean) => {
        writeLocal(key, v ? '1' : '0');
        set(v);
    };
    const setWindow = remember('overviewWindowOpen', setWindowOpen);
    const setBand = remember('overviewBandOpen', setBandOpen);
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
        pane.reveal();
    };
    const openFromOverview = (tid: string) => {
        setOverviewFloatId(tid);
        openThread(tid);
    };
    const dockThread = () => {
        setOverviewFloatId(null);
        placement.setThread('side');
    };
    const closeThread = () => setParams((p) => {
        p.delete('thread');
        return p;
    });
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
            <Box sx={{ maxWidth: 720, mx: 'auto', pt: embedded ? 3 : 8, px: 2 }}>
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

    const canPlace = !pane.narrow;
    const threadEl = threadId ? (
        <ThreadPanel
            // Per thread: its draft and menus start fresh.
            key={threadId}
            projectId={id}
            threadId={threadId}
            thread={selected}
            events={threadEvents}
            running={threadRunning}
            disabled={blocked}
            onBack={closeThread}
            hideTitle={threadAt === 'float'}
            actions={canPlace && overviewAt !== 'top' && (
                <Tooltip title={t(threadAt === 'float' ? 'window.dock' : 'window.float')}>
                    <IconButton
                        size="small"
                        onClick={() => {
                            if (threadAt === 'float') dockThread();
                            else placement.setThread('float');
                        }}
                        aria-label={t(threadAt === 'float' ? 'window.dock' : 'window.float')}
                    ><FloatWindow fontSize="small" /></IconButton>
                </Tooltip>
            )}
        />
    ) : null;
    const overviewEl = (initialTab?: 'threads' | 'library') => (
        <OverviewPane projectId={id} threads={threads} onOpen={openFromOverview} libraryKey={libraryKey} selectedId={threadId} initialTab={initialTab} />
    );
    const threadInSide = !!threadEl && threadAt === 'side';
    // The side pane holds the Overview, or the open thread in its place; with the
    // Overview elsewhere it holds only the thread.
    const side = overviewAt === 'side' ? (threadInSide ? threadEl : overviewEl()) : threadInSide ? threadEl : null;
    const paneToggle = overviewAt === 'window'
        ? { onClick: () => setWindow(!windowOpen), expanded: windowOpen }
        : overviewAt === 'top'
            ? { onClick: () => setBand(!bandOpen), expanded: bandOpen }
            : undefined;

    return (
        <>
            <WorkspaceLayout
                pane={pane}
                headerInsetXs={embedded ? 2 : 6}
                paneLabel={t('project.overview')}
                paneAttention={waiting > 0}
                title={project ? (
                    // One line: the name, then the goal as long as it fits.
                    <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 1.5, minWidth: 0 }}>
                        <Typography variant="h5" noWrap sx={{ flexShrink: { xs: 1, md: 0 }, minWidth: 0, maxWidth: { md: '60%' } }}>
                            {/* No Projects sidebar in a host: the way back to the list is here. */}
                            {embedded && (
                                <>
                                    <Link component={RouterLink} to={routes.list} color="text.secondary" underline="hover">{t('projects.title')}</Link>
                                    <Box component="span" sx={{ color: 'text.disabled', mx: 0.75 }}>/</Box>
                                </>
                            )}
                            {project.name}
                        </Typography>
                        {project.goal && <Typography variant="body2" color="text.secondary" noWrap title={project.goal} sx={{ minWidth: 0, display: { xs: 'none', md: 'block' } }}>{project.goal}</Typography>}
                    </Box>
                ) : <Skeleton width={200} />}
                actions={project && (
                    <>
                        {canPlace && <PlacementMenu overview={placement.overview} thread={placement.thread} onOverview={placement.setOverview} onThread={placement.setThread} overviewFloat={placement.overviewFloat} onOverviewFloat={placement.setOverviewFloat} />}
                        <Tooltip title={project.starred ? t('common.unstar') : t('common.star')}>
                            <IconButton size="small" onClick={() => void patch({ starred: !project.starred })}>{project.starred ? <Star fontSize="small" color="primary" /> : <StarBorder fontSize="small" />}</IconButton>
                        </Tooltip>
                        <Tooltip title={t('projectSettings.title')}>
                            <IconButton size="small" onClick={() => setSettingsTab('general')} aria-label={t('projectSettings.title')}><Settings fontSize="small" /></IconButton>
                        </Tooltip>
                    </>
                )}
                side={side}
                sideAlwaysOpen={overviewAt !== 'side'}
                paneToggle={paneToggle}
                top={overviewAt === 'top' ? (
                    <OverviewBand threads={threads} selectedId={threadId} open={bandOpen} onToggle={() => setBand(!bandOpen)} onOpen={openThread} onLibrary={() => setLibraryWindow(true)} />
                ) : undefined}
                main={messages === null ? (
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
            />

            <FloatingWindow
                open={overviewAt === 'window' && windowOpen}
                title={t('project.overview')}
                storageKey="overview"
                defaultSize={{ w: 460, h: 520 }}
                attention={waiting > 0}
                onClose={() => setWindow(false)}
                onDock={() => placement.setOverview('side')}
            >
                {overviewEl()}
            </FloatingWindow>
            <FloatingWindow
                open={overviewAt !== 'side' && libraryWindow}
                title={t('overview.library')}
                storageKey="library"
                defaultSize={{ w: 460, h: 480 }}
                onClose={() => setLibraryWindow(false)}
            >
                {overviewEl('library')}
            </FloatingWindow>
            <FloatingWindow
                open={threadAt === 'float' && !!threadEl}
                title={selected?.title ?? ''}
                // From the Overview it gets its own spot beside the pane, so the list stays clickable.
                storageKey={floatedFromOverview ? 'thread-overview' : 'thread'}
                defaultSize={{ w: 520, h: 640 }}
                clearRight={floatedFromOverview && pane.open ? pane.width : 0}
                attention={selected?.status === 'waiting'}
                onClose={closeThread}
                onDock={dockThread}
            >
                {threadEl}
            </FloatingWindow>

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
        </>
    );
}
