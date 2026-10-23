import { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Box, Button, IconButton, Link, Skeleton, Tooltip, Typography } from '@mui/material';
import { Link as RouterLink, Navigate, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { api, isFolder, type components, unwrap, type Project, type Thread, type ThreadEvent } from '../client';
import { ConversationPane } from '../components/project/ConversationPane';
import { OverviewPane } from '../components/project/OverviewPane';
import { ModelPicker } from '../components/ModelPicker';
import { PermissionModeSelect } from '../components/workdir/PermissionModeSelect';
import { ProjectSettingsDialog, type SettingsTab } from '../components/project/ProjectSettingsDialog';
import { ProjectImTags, useProjectIm } from '../components/im/ProjectImBindings';
import { IM_BINDINGS_CHANGED } from '../components/im/useImBindings';
import { ThreadPanel } from '../components/project/ThreadPanel';
import { FloatingWindow } from '../components/workspace/FloatingWindow';
import { OverviewBand } from '../components/workspace/OverviewBand';
import { PlacementMenu } from '../components/workspace/PlacementMenu';
import { WorkspaceLayout, useWorkspacePane } from '../components/workspace/WorkspaceLayout';
import { FloatWindow, Pause, Settings, Star, StarBorder } from '../components/icons';
import { useNotify } from '../hooks/useNotify';
import { useProjectEvents, type ProjectEvent } from '../hooks/useProjectEvents';
import { useTranscript } from '../hooks/useTranscript';
import { usePlacement } from '../hooks/usePlacement';
import { useProjectsHost } from '../host/ProjectsHost';
import { readLocal, writeLocal } from '../utils/storage';
import { needsUser } from '../components/project/threadStatus';


// The permission modes a project can give its threads (the server refuses others).
type ThreadMode = NonNullable<components['schemas']['UpdateProjectRequest']['thread_permission_mode']>;
const THREAD_MODES = ['default', 'acceptEdits', 'auto', 'bypassPermissions'];

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
    // The project conversation: the main thread's events.
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const pane = useWorkspacePane();
    // IM chats bound to this project (null when the host has no IM).
    const im = useProjectIm(project?.main_thread_id ? { threadId: project.main_thread_id, projectId: project.id, name: project.name } : null);
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

    const mainThreadId = project?.main_thread_id ?? '';
    // The conversation (the main thread) and the open thread: their latest
    // entries, then only what changes (useTranscript).
    const conversation = useTranscript(mainThreadId);
    const { apply: applyMain, edit: editMain } = conversation;
    const messages = conversation.events;
    const opened = useTranscript(threadId);
    const { apply: applyOpened } = opened;
    const threadEvents = opened.events;

    const load = useCallback(async () => {
        try {
            const d = await unwrap(api.GET('/api/v1/projects/{id}', { params: { path: { id } } }));
            setProject(d.project);
            setThreads(d.threads);
            setBusy(d.coordinator_busy);
            setError(null);
        } catch (e) {
            setError((e as Error).message);
        }
    }, [id]);

    const loadThread = useCallback(async (tid: string) => {
        try {
            const d = await unwrap(api.GET('/api/v1/threads/{id}', { params: { path: { id: tid } } }));
            setThreads((list) => upsert(list, d.thread));
        } catch (e) {
            notify((e as Error).message, 'error');
        }
    }, [notify]);

    useEffect(() => {
        setProject(null);
        setThreads([]);
        setBusy(false);
        void load();
    }, [load]);

    useEffect(() => {
        if (threadId) void loadThread(threadId);
    }, [threadId, loadThread]);

    const onEvent = useCallback((ev: ProjectEvent) => {
        switch (ev.type) {
            case 'message_delta': {
                const d = ev.data as { id: string; text: string };
                editMain((list) => list.map((m) => (m.id === d.id ? { ...m, content: m.content + d.text } : m)));
                break;
            }
            case 'thread': {
                const th = ev.data as Thread;
                // The main thread is the conversation, not a row of the Overview.
                if (th.id === mainThreadId) break;
                setThreads((list) => upsert(list, th));
                if (th.status !== 'working' && th.status !== 'waiting') setLibraryKey((k) => k + 1);
                // No refresh(): the shared list's own stream covers thread events.
                break;
            }
            case 'thread_event': {
                const e = ev.data as ThreadEvent;
                if (e.thread_id === mainThreadId) applyMain(e);
                else if (e.thread_id === threadId) applyOpened(e);
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
    }, [threadId, mainThreadId, applyMain, editMain, applyOpened]);

    useProjectEvents(id, onEvent, () => {
        void load();
        conversation.catchUp().catch(() => undefined);
        if (threadId) {
            void loadThread(threadId);
            opened.catchUp().catch(() => undefined);
        }
    });

    const selected = threads.find((th) => th.id === threadId);
    // A turn is in flight while the thread works or waits on a request it
    // raised (approvals only exist inside a running turn).
    const threadRunning = selected?.status === 'working'
        || (selected?.status === 'waiting' && !!threadEvents?.some((e) => (e.kind === 'approval' || e.kind === 'ask') && e.payload?.includes('"state":"pending"')));

    const threadMap = useMemo(() => new Map(threads.map((th) => [th.id, th])), [threads]);
    const waiting = threads.filter((th) => needsUser(th.status)).length;

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
    const patch = async (body: Pick<components['schemas']['UpdateProjectRequest'], 'starred' | 'paused' | 'archived' | 'coordinator_model' | 'thread_model' | 'thread_permission_mode'>) => {
        try {
            setProject(await unwrap(api.PATCH('/api/v1/projects/{id}', { params: { path: { id } }, body })));
            // Archiving unbinds the project's IM chats.
            if (body.archived) window.dispatchEvent(new Event(IM_BINDINGS_CHANGED));
            refresh();
        } catch (e) {
            notify((e as Error).message, 'error');
        }
    };

    // A folder is a project too, but its page is the folder page.
    if (project && isFolder(project)) {
        const to = routes.folder(project.path);
        return <Navigate to={threadId ? `${to}${to.includes('?') ? '&' : '?'}session=${encodeURIComponent(threadId)}` : to} replace />;
    }

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
            more={opened.more}
            onLoadEarlier={opened.loadEarlier}
            subagents={opened.subagents}
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
                        {/* Wide screens keep the bound chats in the header; narrow ones get their own row below it. */}
                        {im && <Box sx={{ display: { xs: 'none', md: 'flex' }, minWidth: 0, flexShrink: 1 }}><ProjectImTags im={im} /></Box>}
                        {canPlace && <PlacementMenu overview={placement.overview} thread={placement.thread} onOverview={placement.setOverview} onThread={placement.setThread} overviewFloat={placement.overviewFloat} onOverviewFloat={placement.setOverviewFloat} />}
                        <Tooltip title={project.starred ? t('common.unstar') : t('common.star')}>
                            <IconButton size="small" onClick={() => void patch({ starred: !project.starred })}>{project.starred ? <Star fontSize="small" color="primary" /> : <StarBorder fontSize="small" />}</IconButton>
                        </Tooltip>
                        <Tooltip title={t('projectSettings.title')}>
                            <IconButton size="small" onClick={() => setSettingsTab('general')} aria-label={t('projectSettings.title')}><Settings fontSize="small" /></IconButton>
                        </Tooltip>
                    </>
                )}
                notice={im && (
                    <Box sx={{ display: { xs: 'flex', md: 'none' }, px: 2, py: 1, borderBottom: 1, borderColor: 'divider' }}>
                        <ProjectImTags im={im} maxVisible={99} showLabel />
                    </Box>
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
                        projectId={id}
                        messages={messages}
                        more={conversation.more}
                        onLoadEarlier={conversation.loadEarlier}
                        threads={threadMap}
                        busy={busy}
                        disabled={blocked || !project}
                        disabledReason={banner}
                        threadId={mainThreadId}
                        model={project?.coordinator_model}
                        onModelChange={(m) => void patch({ coordinator_model: m })}
                        composerContext={project && (
                            // What the threads it starts run with; the coordinator's own model sits beside Send.
                            <>
                                <PermissionModeSelect
                                    value={project.thread_permission_mode || 'acceptEdits'}
                                    modes={THREAD_MODES}
                                    disabled={blocked}
                                    onChange={(m) => void patch({ thread_permission_mode: m as ThreadMode })}
                                />
                                <ModelPicker
                                    label={t('conversation.threadModel')}
                                    value={project.thread_model}
                                    disabled={blocked}
                                    onChange={(m) => void patch({ thread_model: m })}
                                />
                            </>
                        )}
                        onOpenThread={openThread}
                        chatTitle={im?.chatTitle}
                        onSend={async (text, attachments) => {
                            try {
                                await unwrap(api.POST('/api/v1/threads/{id}/messages', { params: { path: { id: mainThreadId } }, body: { content: text, attachments: attachments.map((a) => a.path) } }));
                                setBusy(true);
                                // The message shows at once; the reply streams in over the event stream.
                                await conversation.catchUp().catch(() => undefined);
                                return true;
                            } catch (e) {
                                notify((e as Error).message, 'error');
                                return false;
                            }
                        }}
                        onStartSuggestions={async (mid, indexes) => {
                            try {
                                const m = await unwrap(api.POST('/api/v1/threads/{id}/events/{eid}/start', { params: { path: { id: mainThreadId, eid: mid } }, body: { indexes } }));
                                conversation.apply(m);
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
                attention={!!selected && needsUser(selected.status)}
                onClose={closeThread}
                onDock={dockThread}
            >
                {threadEl}
            </FloatingWindow>

            {im?.dialog}

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
