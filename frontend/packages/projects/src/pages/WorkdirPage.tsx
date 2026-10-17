import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Alert, Box, Button, IconButton, Skeleton, Tooltip, Typography } from '@mui/material';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { api, unwrap, type SessionDetail, type Thread, type ThreadEvent } from '../client';
import { AddWorkdirDialog } from '../components/workdir/AddWorkdirDialog';
import { NewSessionView } from '../components/workdir/NewSessionView';
import { loadPermissionModes, PermissionModeSelect } from '../components/workdir/PermissionModeSelect';
import { useWorkdirs } from '../components/workdir/useWorkdirs';
import { ThreadPanel } from '../components/project/ThreadPanel';
import { StatusGroups, type StatusGroup } from '../components/workspace/StatusGroups';
import { WorkspaceLayout, useWorkspacePane } from '../components/workspace/WorkspaceLayout';
import { Add, Close } from '../components/icons';
import { useNotify } from '../hooks/useNotify';
import { useSessionEvents, type ProjectEvent } from '../hooks/useProjectEvents';
import { useProjectsHost } from '../host/ProjectsHost';
import { folderName, timeAgo } from '../utils/format';

/** Replace the item with the same id, else add it (sessions newest first, transcript entries last). */
function upsert<T extends { id: string }>(list: T[], item: T, last = false): T[] {
    const i = list.findIndex((x) => x.id === item.id);
    if (i < 0) return last ? [...list, item] : [item, ...list];
    const next = list.slice();
    next[i] = item;
    return next;
}

const GROUPS: { key: string; color: StatusGroup['color']; collapsed?: boolean }[] = [
    { key: 'waiting', color: 'warning' },
    { key: 'working', color: 'info' },
    { key: 'idle', color: 'default' },
    { key: 'resolved', color: 'default', collapsed: true },
];

/**
 * A working folder: Claude Code sitting in a plain folder, no worktree or
 * coordinator. The side pane lists the folder's sessions the way a project
 * lists its threads (what needs you first); the main area is the open
 * session, or the prompt for a new one. Without a folder it lists them all.
 */
export default function WorkdirPage() {
    const { t, i18n } = useTranslation('projects');
    const notify = useNotify();
    const { onProjectsChanged: refresh, embedded } = useProjectsHost();
    const [params, setParams] = useSearchParams();
    const scope = params.get('path') ?? '';
    const sessionId = params.get('session') ?? '';
    const pane = useWorkspacePane('folder.');
    const { workdirs, reload: reloadWorkdirs } = useWorkdirs();

    const [sessions, setSessions] = useState<Thread[] | null>(null);
    const [events, setEvents] = useState<ThreadEvent[] | null>(null);
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
        const n = ++sessionsReq.current;
        try {
            const r = await unwrap(api.GET('/api/v1/projects/sessions', { params: { query: { folder: scope || undefined } } }));
            if (n !== sessionsReq.current) return;
            setSessions(r.sessions);
            setError(null);
        } catch (e) {
            if (n === sessionsReq.current) setError((e as Error).message);
        }
    }, [scope]);

    const loadSession = useCallback(async (sid: string) => {
        const n = ++sessionReq.current;
        try {
            const d: SessionDetail = await unwrap(api.GET('/api/v1/projects/sessions/{sid}', { params: { path: { sid } } }));
            if (n !== sessionReq.current) return;
            setEvents(d.events);
            setSessions((list) => upsert(list ?? [], d.session));
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

    useEffect(() => {
        setSessions(null);
        void loadSessions();
    }, [loadSessions]);

    useEffect(() => {
        setEvents(null);
        if (sessionId) void loadSession(sessionId);
        else sessionReq.current++;
    }, [sessionId, loadSession]);

    useSessionEvents((ev: ProjectEvent) => {
        if (ev.type === 'thread') {
            const th = ev.data as Thread;
            if (!scope || th.worktree === scope) setSessions((list) => upsert(list ?? [], th));
            refresh();
            reloadWorkdirs();
        } else if (ev.type === 'thread_event') {
            const e = ev.data as ThreadEvent;
            if (e.thread_id === sessionId) setEvents((list) => (list ? upsert(list, e, true) : list));
        }
    }, () => {
        void loadSessions();
        if (sessionId) void loadSession(sessionId);
    });

    const selected = sessions?.find((s) => s.id === sessionId);
    const running = selected?.status === 'working'
        || (selected?.status === 'waiting' && !!events?.some((e) => (e.kind === 'approval' || e.kind === 'ask') && e.payload?.includes('"state":"pending"')));

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
    const known = workdirs?.find((w) => w.path === scope);
    const removable = !!known?.saved;

    const create = async (folder: string, content: string, permission_mode: string): Promise<boolean> => {
        try {
            const s = await unwrap(api.POST('/api/v1/projects/sessions', { body: { folder, content, permission_mode: permission_mode || undefined } }));
            // Picked another folder than the one open: go to that folder.
            if (scope && s.worktree !== scope) {
                go({ path: s.worktree, session: s.id });
            } else {
                setSessions((list) => upsert(list ?? [], s));
                go({ ...(scope && { path: scope }), session: s.id });
            }
            refresh();
            reloadWorkdirs();
            return true;
        } catch (e) {
            notify((e as Error).message, 'error');
            return false;
        }
    };

    const setMode = async (mode: string) => {
        if (!selected) return;
        try {
            const s = await unwrap(api.PUT('/api/v1/projects/sessions/{sid}/permission-mode', { params: { path: { sid: selected.id } }, body: { permission_mode: mode } }));
            setSessions((list) => upsert(list ?? [], s));
        } catch (e) {
            notify((e as Error).message, 'error');
        }
    };

    const removeFolder = async () => {
        try {
            await unwrap(api.DELETE('/api/v1/projects/workdirs', { params: { query: { path: scope } } }));
            refresh();
            reloadWorkdirs();
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
        subtitle: [scope ? '' : folderName(s.worktree), s.status === 'waiting' ? s.wait_reason : ''].filter(Boolean).join(' · ') || undefined,
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
                        {removable && (
                            <Tooltip title={t('workdir.remove')}>
                                <IconButton size="small" aria-label={t('workdir.remove')} onClick={() => void removeFolder()}><Close fontSize="small" /></IconButton>
                            </Tooltip>
                        )}
                        <Button size="small" startIcon={<Add />} onClick={() => openNew()}>{t('session.new')}</Button>
                    </>
                )}
                notice={error && (
                    <Alert severity="warning" action={<Button color="inherit" size="small" onClick={() => void loadSessions()}>{t('common.retry')}</Button>}>
                        {t('session.refreshFailed')}
                        <Box component="span" sx={{ display: 'block', fontSize: fontSizes.md }}>{error}</Box>
                    </Alert>
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
                        projectId=""
                        threadId={sessionId}
                        thread={selected}
                        events={events}
                        running={running}
                        disabled={false}
                        onBack={() => openNew()}
                        composerContext={selected && (
                            <PermissionModeSelect value={selected.permission_mode ?? ''} modes={modes} onChange={(m) => void setMode(m)} />
                        )}
                    />
                ) : (
                    <NewSessionView
                        // Remount per folder so the form starts from that folder.
                        key={scope}
                        initialFolder={scope || undefined}
                        folders={workdirs ?? []}
                        permissionModes={modes}
                        onCreate={create}
                        onAddFolder={() => setAdding(true)}
                    />
                )}
            />
            {adding && (
                <AddWorkdirDialog
                    onClose={() => setAdding(false)}
                    onAdded={(w) => {
                        refresh();
                        reloadWorkdirs();
                        go({ path: w.path, new: '1' });
                    }}
                />
            )}
        </>
    );
}
