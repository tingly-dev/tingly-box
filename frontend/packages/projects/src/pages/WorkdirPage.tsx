import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Alert, Box, Button, IconButton, Skeleton, Tooltip, Typography } from '@mui/material';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { api, unwrap, type Attachment, type BackgroundTask, type Project, type Thread, type ThreadEvent } from '../client';
import { AddWorkdirDialog } from '../components/workdir/AddWorkdirDialog';
import { NewSessionView } from '../components/workdir/NewSessionView';
import { loadPermissionModes, PermissionModeSelect } from '../components/workdir/PermissionModeSelect';
import { uploadAttachments } from '../components/attachments';
import { ProjectImTags, useProjectIm } from '../components/im/ProjectImBindings';
import { IM_BINDINGS_CHANGED } from '../components/im/useImBindings';
import { ThreadPanel } from '../components/project/ThreadPanel';
import { StatusGroups, type StatusGroup } from '../components/workspace/StatusGroups';
import { WorkspaceLayout, useWorkspacePane } from '../components/workspace/WorkspaceLayout';
import { Add, Close } from '../components/icons';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { useNotify } from '../hooks/useNotify';
import { useProjectEvents, type ProjectEvent } from '../hooks/useProjectEvents';
import { useProjectList } from '../hooks/useProjectList';
import { useProjectsHost, type SessionLaunch } from '../host/ProjectsHost';
import { folderName, timeAgo } from '../utils/format';

/** Replace the item with the same id, else add it (sessions newest first, transcript entries last). */
function upsert<T extends { id: string }>(list: T[], item: T, last = false): T[] {
    const i = list.findIndex((x) => x.id === item.id);
    if (i < 0) return last ? [...list, item] : [item, ...list];
    const next = list.slice();
    next[i] = item;
    return next;
}

/** Whether a task event changes the set of running background tasks (progress does not). */
function changesLiveTasks(e: ThreadEvent): boolean {
    try {
        const ev = (JSON.parse(e.payload ?? '{}') as { event?: string }).event;
        return ev === 'background_tasks_changed' || ev === 'task_notification' || ev === 'task_completed';
    } catch {
        return false;
    }
}

const GROUPS: { key: string; color: StatusGroup['color']; collapsed?: boolean }[] = [
    { key: 'waiting', color: 'warning' },
    { key: 'working', color: 'info' },
    { key: 'idle', color: 'default' },
    { key: 'resolved', color: 'default', collapsed: true },
];

/**
 * A working folder (a project of type folder): Claude Code sitting in a plain
 * folder, no worktree or coordinator. The side pane lists the folder's threads
 * the way a project lists its own (what needs you first); the main area is
 * the open thread, or the prompt for a new one. Without a folder it lists
 * them all. A folder has no goal, memory, library or review: only threads.
 */
export default function WorkdirPage() {
    const { t, i18n } = useTranslation('projects');
    const notify = useNotify();
    const { onProjectsChanged: refresh, embedded, session: slots } = useProjectsHost();
    const [params, setParams] = useSearchParams();
    const scope = params.get('path') ?? '';
    const sessionId = params.get('session') ?? '';
    const pane = useWorkspacePane('folder.');
    // The folders are the folder-typed projects of the one project list (the
    // sidebar's too); its all-projects stream carries thread / project frames
    // only. Transcript entries (thread_event) come on the open thread's own
    // project stream, subscribed below.
    const onEvent = useRef<(ev: ProjectEvent) => void>(() => undefined);
    const onOpen = useRef<() => void>(() => undefined);
    const onThreadEvent = useRef<(ev: ProjectEvent) => void>(() => undefined);
    const { folders, reload: reloadList } = useProjectList(true, (ev) => onEvent.current(ev), () => onOpen.current());
    const folder = scope ? folders?.find((f) => f.path === scope) : undefined;
    // Which folders' threads this page lists, as a stable key: the open folder, or every folder with threads.
    const listed = !folders ? null
        : scope ? (folder ? [folder.id] : [])
            : folders.filter((f) => f.threads.waiting + f.threads.working + f.threads.ready + f.threads.idle + f.threads.resolved > 0).map((f) => f.id);
    const listedKey = listed?.join(',') ?? null;

    const [sessions, setSessions] = useState<Thread[] | null>(null);
    const [events, setEvents] = useState<ThreadEvent[] | null>(null);
    const [liveTasks, setLiveTasks] = useState<BackgroundTask[]>([]);
    const [modes, setModes] = useState<string[]>([]);
    // The add dialog lives in the URL, so a link (the sidebar's "add") opens it.
    const adding = params.get('add') === '1';
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        loadPermissionModes().then(setModes).catch(() => {
            // Not critical: the picker offers only the default.
        });
    }, []);

    // Only the latest request lands: switching folders or sessions quickly
    // must not leave an older answer on screen.
    const sessionsReq = useRef(0);
    const sessionReq = useRef(0);

    const loadSessions = useCallback(async () => {
        if (listedKey === null) return;
        const n = ++sessionsReq.current;
        try {
            const lists = await Promise.all(listedKey.split(',').filter(Boolean).map((id) => unwrap(api.GET('/api/v1/projects/{id}/threads', { params: { path: { id } } }))));
            if (n !== sessionsReq.current) return;
            setSessions(lists.flatMap((r) => r.threads).sort((a, b) => b.updated_at.localeCompare(a.updated_at)));
            setError(null);
        } catch (e) {
            if (n === sessionsReq.current) setError((e as Error).message);
        }
    }, [listedKey]);

    const loadSession = useCallback(async (sid: string) => {
        const n = ++sessionReq.current;
        try {
            const [d, ev] = await Promise.all([
                unwrap(api.GET('/api/v1/threads/{id}', { params: { path: { id: sid } } })),
                unwrap(api.GET('/api/v1/threads/{id}/events', { params: { path: { id: sid } } })),
            ]);
            if (n !== sessionReq.current) return;
            setEvents(ev.events);
            setLiveTasks(d.background_tasks ?? []);
            setSessions((list) => upsert(list ?? [], d.thread));
        } catch (e) {
            if (n !== sessionReq.current) return;
            notify((e as Error).message, 'error');
            // Gone or unreadable: back to the folder's new-session prompt
            // rather than a panel that never loads.
            setParams((p) => {
                const next = new URLSearchParams(p);
                next.delete('session');
                return next;
            }, { replace: true });
        }
    }, [notify, setParams]);

    // The live background tasks come with the thread, not its transcript (the stream carries that).
    const loadTasks = useCallback(async (sid: string) => {
        const n = sessionReq.current;
        try {
            const d = await unwrap(api.GET('/api/v1/threads/{id}', { params: { path: { id: sid } } }));
            if (n !== sessionReq.current) return;
            setLiveTasks(d.background_tasks ?? []);
            setSessions((list) => upsert(list ?? [], d.thread));
        } catch {
            // Keep what the panel shows; the next change or reconnect reads it again.
        }
    }, []);

    // Another folder starts from an empty list; the same folder (or all of them) keeps what it shows while it refreshes.
    useEffect(() => {
        setSessions(null);
    }, [scope]);

    useEffect(() => {
        void loadSessions();
    }, [loadSessions]);

    useEffect(() => {
        setEvents(null);
        setLiveTasks([]);
        if (sessionId) void loadSession(sessionId);
        else sessionReq.current++;
    }, [sessionId, loadSession]);

    // The list's stream calls these; they always see this render's folder, thread and loaders.
    useEffect(() => {
        onEvent.current = (ev: ProjectEvent) => {
            if (ev.type === 'thread') {
                const th = ev.data as Thread;
                // Only the folders this page lists (the stream carries every project's).
                const mine = scope ? th.project_id === folder?.id : !!folders?.some((f) => f.id === th.project_id);
                // The list stream already refreshes the folder list for this event.
                if (mine) setSessions((list) => upsert(list ?? [], th));
            }
        };
        onThreadEvent.current = (ev: ProjectEvent) => {
            if (ev.type !== 'thread_event') return;
            const e = ev.data as ThreadEvent;
            if (e.thread_id !== sessionId) return;
            setEvents((list) => (list ? upsert(list, e, true) : list));
            if (e.kind === 'task' && changesLiveTasks(e)) void loadTasks(sessionId);
        };
        onOpen.current = () => {
            void loadSessions();
            if (sessionId) void loadSession(sessionId);
        };
    });

    const selected = sessions?.find((s) => s.id === sessionId);
    // The open thread's project stream (the all-projects one never carries thread_event). It reconnects on its own;
    // on a reconnect the thread is re-read for anything missed.
    useProjectEvents(sessionId ? (selected?.project_id ?? folder?.id ?? '') : '', (ev) => onThreadEvent.current(ev), () => {
        if (sessionId) void loadSession(sessionId);
    });
    const running = selected?.status === 'working'
        || (selected?.status === 'waiting' && !!events?.some((e) => (e.kind === 'approval' || e.kind === 'ask') && e.payload?.includes('"state":"pending"')));

    // IM chats bound to the open thread: a folder is bound by one of its threads, never as a project.
    const im = useProjectIm(selected ? { threadId: selected.id, name: selected.title } : null);

    const go = (next: Record<string, string>) => setParams(next);
    const openSession = (id: string) => {
        go({ ...(scope && { path: scope }), session: id });
        pane.setDrawerOpen(false);
    };
    const openNew = (folder = scope) => go(folder ? { path: folder, new: '1' } : { new: '1' });
    const setAdding = (open: boolean) => setParams((p) => {
        const next = new URLSearchParams(p);
        if (open) next.set('add', '1');
        else next.delete('add');
        return next;
    });
    // A folder in the list can be taken off the sidebar; its threads stay (archive, not delete).
    const removable = !!folder;

    // The folder's project, opened first if it is new (the server returns the existing one otherwise).
    const openFolder = (path: string): Promise<Project> => unwrap(api.POST('/api/v1/projects', { body: { type: 'folder', path } }));

    // Uploads need a folder project to hang the files on (the server keeps a
    // draft area per folder), so attaching opens the folder first. Side effect,
    // accepted: a draft that is never sent still leaves the folder opened (or
    // un-archived) in the list, which is why the list is reloaded here and
    // Attach stays disabled until a folder is chosen. A draft owner that is
    // independent of the folder would need a backend change.
    const attach = async (path: string, files: File[]): Promise<Attachment[]> => {
        const f = await openFolder(path);
        reloadList();
        return uploadAttachments({ draftFolderId: f.id }, files);
    };

    const create = async (path: string, content: string, permission_mode: string, launch: SessionLaunch, attachments: Attachment[]): Promise<boolean> => {
        try {
            const f = await openFolder(path);
            const s = await unwrap(api.POST('/api/v1/projects/{id}/threads', {
                params: { path: { id: f.id } },
                body: { content, attachments: attachments.map((a) => a.path), permission_mode: permission_mode || undefined, profile: launch.profile || undefined, model: launch.model || undefined },
            }));
            // Picked another folder than the one open: go to that folder.
            if (scope && s.cwd !== scope) {
                go({ path: s.cwd, session: s.id });
            } else {
                setSessions((list) => upsert(list ?? [], s));
                go({ ...(scope && { path: scope }), session: s.id });
            }
            refresh();
            reloadList();
            return true;
        } catch (e) {
            notify((e as Error).message, 'error');
            return false;
        }
    };

    const setMode = async (mode: string) => {
        if (!selected) return;
        try {
            const s = await unwrap(api.PUT('/api/v1/threads/{id}/permission-mode', { params: { path: { id: selected.id } }, body: { permission_mode: mode } }));
            setSessions((list) => upsert(list ?? [], s));
        } catch (e) {
            notify((e as Error).message, 'error');
        }
    };

    // A profile change goes first: it may clear a model the profile cannot run.
    const setLaunch = async (next: SessionLaunch) => {
        if (!selected) return;
        const path = { params: { path: { id: selected.id } } };
        try {
            let s = selected;
            if (next.profile !== (s.profile ?? '')) {
                // The server resets the model with the profile.
                s = await unwrap(api.PUT('/api/v1/threads/{id}/profile', { ...path, body: { profile: next.profile } }));
                setSessions((list) => upsert(list ?? [], s));
            }
            if (next.model !== (s.model ?? '')) {
                s = await unwrap(api.PUT('/api/v1/threads/{id}/model', { ...path, body: { model: next.model } }));
                setSessions((list) => upsert(list ?? [], s));
            }
        } catch (e) {
            notify((e as Error).message, 'error');
        }
    };

    // Threads a removal would stop: the ones working or waiting on a request.
    const stopCount = folder ? folder.threads.working + folder.threads.waiting : 0;
    const [confirmRemove, setConfirmRemove] = useState(false);

    const removeFolder = async () => {
        try {
            if (!folder) return;
            await unwrap(api.DELETE('/api/v1/projects/{id}', { params: { path: { id: folder.id } } }));
            // Removing a folder archives it, which unbinds its IM chats.
            window.dispatchEvent(new Event(IM_BINDINGS_CHANGED));
            refresh();
            reloadList();
            go({});
        } catch (e) {
            notify((e as Error).message, 'error');
        }
    };

    const groups: StatusGroup[] = GROUPS.map((g) => ({ key: g.key, label: t(`session.group.${g.key}`), color: g.color, collapsed: g.collapsed }));
    const rows = useMemo(() => (sessions ?? []).map((s) => ({
        id: s.id,
        group: s.status === 'ready' ? 'idle' : s.status,
        title: s.title,
        subtitle: [scope ? '' : folderName(s.cwd), s.status === 'waiting' ? s.wait_reason : ''].filter(Boolean).join(' · ') || undefined,
        meta: timeAgo(s.updated_at, i18n.language),
        selected: s.id === sessionId,
    })), [sessions, scope, sessionId, i18n.language]);
    const waitingHere = (sessions ?? []).some((s) => s.status === 'waiting');

    return (
        <>
            <WorkspaceLayout
                pane={pane}
                headerInsetXs={embedded ? 2 : 6}
                paneLabel={t('session.sessions')}
                paneAttention={waitingHere}
                title={scope ? (
                    <>
                        <Typography variant="h5" noWrap>{folderName(scope)}</Typography>
                        <Typography variant="caption" color="text.secondary" noWrap component="div" sx={{ fontFamily: fontMono }}>{scope}</Typography>
                    </>
                ) : <Typography variant="h5" noWrap>{t('workdir.title')}</Typography>}
                actions={(
                    <>
                        {im && <Box sx={{ display: { xs: 'none', md: 'flex' }, minWidth: 0, flexShrink: 1 }}><ProjectImTags im={im} /></Box>}
                        {removable && (
                            <Tooltip title={t('workdir.remove')}>
                                <IconButton size="small" aria-label={t('workdir.remove')} onClick={() => (stopCount > 0 ? setConfirmRemove(true) : void removeFolder())}><Close fontSize="small" /></IconButton>
                            </Tooltip>
                        )}
                        <Button size="small" startIcon={<Add />} onClick={() => openNew()}>{t('session.new')}</Button>
                    </>
                )}
                notice={(error || im) && (
                    <>
                        {error && (
                            <Alert severity="warning" action={<Button color="inherit" size="small" onClick={() => void loadSessions()}>{t('common.retry')}</Button>}>
                                {t('session.refreshFailed')}
                                <Box component="span" sx={{ display: 'block', fontSize: fontSizes.md }}>{error}</Box>
                            </Alert>
                        )}
                        {im && (
                            <Box sx={{ display: { xs: 'flex', md: 'none' }, px: 2, py: 1, borderBottom: 1, borderColor: 'divider' }}>
                                <ProjectImTags im={im} maxVisible={99} showLabel />
                            </Box>
                        )}
                    </>
                )}
                side={(
                    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}>
                        <Box sx={{ display: 'flex', alignItems: 'center', px: 2, minHeight: 44, borderBottom: 1, borderColor: 'divider' }}>
                            <Typography variant="body2" sx={{ fontWeight: 600 }}>{t('session.sessions')}</Typography>
                        </Box>
                        <Box sx={{ flex: 1, overflowY: 'auto', px: 0.5 }}>
                            {sessions === null
                                ? <Box sx={{ p: 2 }}><Skeleton height={36} /><Skeleton height={36} /></Box>
                                : <StatusGroups groups={groups} rows={rows} onOpen={openSession} empty={t('session.none')} />}
                        </Box>
                    </Box>
                )}
                main={sessionId ? (
                    <ThreadPanel
                        key={sessionId}
                        projectId={selected?.project_id ?? folder?.id ?? ''}
                        folder
                        threadId={sessionId}
                        thread={selected}
                        events={events}
                        running={running}
                        disabled={false}
                        onBack={() => openNew()}
                        liveTasks={liveTasks}
                        onRefresh={() => void loadSession(sessionId)}
                        composerContext={selected && (
                            <>
                                {slots?.launchPickers?.({ value: { profile: selected.profile ?? '', model: selected.model ?? '' }, onChange: (n) => void setLaunch(n) })}
                                <PermissionModeSelect value={selected.permission_mode ?? ''} modes={modes} onChange={(m) => void setMode(m)} />
                            </>
                        )}
                        belowComposer={selected && events && slots?.statusLine?.({ session: selected, events })}
                    />
                ) : (
                    <NewSessionView
                        // Remount per folder so the form starts from that folder.
                        key={scope}
                        initialFolder={scope || undefined}
                        folders={(folders ?? []).map((f) => f.path ?? '')}
                        permissionModes={modes}
                        onCreate={create}
                        onAddFolder={() => setAdding(true)}
                        onAttach={attach}
                    />
                )}
            />
            {im?.dialog}
            <ConfirmDialog
                open={confirmRemove}
                title={t('workdir.removeTitle')}
                message={t('workdir.removeRunning', { count: stopCount, name: folder ? folderName(folder.path ?? '') : '' })}
                confirmLabel={t('workdir.removeConfirm')}
                danger
                onClose={() => setConfirmRemove(false)}
                onConfirm={() => void removeFolder()}
            />
            {adding && (
                <AddWorkdirDialog
                    onClose={() => setAdding(false)}
                    onAdded={(w) => {
                        refresh();
                        reloadList();
                        go({ path: w.path ?? '', new: '1' });
                    }}
                />
            )}
        </>
    );
}
