import { useEffect, useState } from 'react';
import { Box, ButtonBase, IconButton, Typography } from '@mui/material';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { NavLink } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type ProjectSummary, type Thread } from '../client';
import { folderName, timeAgo } from '../utils/format';
import { readLocal, writeLocal } from '../utils/storage';
import { ChevronRight, ExpandMore, Folder, Stack, Star, Terminal } from './icons';
import { needsUser, stateColor, ThreadStateMark } from './project/threadStatus';

/** Sessions shown per open folder before "Show older"; running and waiting ones always show. */
const VISIBLE = 5;

const LABEL_SX = { fontSize: fontSizes.md, fontWeight: 600, lineHeight: 1.3 } as const;
/** Sessions line up with the node's name: icon column plus its gap. */
const SESSION_INDENT = 4.5;
const ROW_SX = {
    display: 'flex', alignItems: 'center', gap: 1, mx: 1, pl: 2, pr: 1.5, minHeight: 32, borderRadius: 1,
    textDecoration: 'none', color: 'text.primary',
    '&:hover': { backgroundColor: 'action.hover' },
} as const;

const rank = (th: Thread) => (needsUser(th.status) ? 0 : th.status === 'working' ? 1 : th.status === 'resolved' ? 3 : 2);

/** Needs-you first, then running, then the latest; archived ones last. */
export const sortSessions = (threads: Thread[]) =>
    [...threads].sort((a, b) => rank(a) - rank(b) || b.updated_at.localeCompare(a.updated_at));

/** A folder's sessions, read again when its counts change. Only while the folder is open. */
function useFolderSessions(folder: ProjectSummary, open: boolean): Thread[] {
    const [threads, setThreads] = useState<Thread[]>([]);
    const c = folder.threads;
    const signature = `${c.working}/${c.waiting}/${c.failed}/${c.idle}/${c.ready}`;
    useEffect(() => {
        if (!open) return;
        let alive = true;
        unwrap(api.GET('/api/v1/projects/{id}/threads', { params: { path: { id: folder.id } } }))
            .then((r) => alive && setThreads(r.threads ?? []))
            .catch(() => {
                // The folder row still works without its sessions.
            });
        return () => {
            alive = false;
        };
    }, [folder.id, open, signature]);
    return threads;
}

/** The background tasks a running session has, only while it runs. */
function useBackgroundTasks(thread: Thread): { task_id: string; description: string }[] {
    const [tasks, setTasks] = useState<{ task_id: string; description: string }[]>([]);
    const running = thread.status === 'working';
    useEffect(() => {
        if (!running) {
            setTasks([]);
            return;
        }
        let alive = true;
        unwrap(api.GET('/api/v1/threads/{id}', { params: { path: { id: thread.id } } }))
            .then((d) => alive && setTasks(d.background_tasks ?? []))
            .catch(() => {
                // Without the tasks the session row is still right.
            });
        return () => {
            alive = false;
        };
    }, [thread.id, running, thread.updated_at]);
    return tasks;
}

function SessionRow({ thread, to, selected, onNavigate }: { thread: Thread; to: string; selected: boolean; onNavigate?: () => void }) {
    const { i18n } = useTranslation('projects');
    const tasks = useBackgroundTasks(thread);
    return (
        <>
            <Box
                component={NavLink}
                to={to}
                onClick={onNavigate}
                title={thread.title}
                aria-current={selected ? 'page' : undefined}
                sx={{ ...ROW_SX, pl: SESSION_INDENT, ...(selected && { backgroundColor: 'action.selected' }) }}
            >
                <Typography noWrap sx={{ flex: 1, minWidth: 0, fontSize: fontSizes.base, fontWeight: 500, color: 'inherit' }}>{thread.title}</Typography>
                <Typography component="span" sx={{ flexShrink: 0, fontSize: fontSizes.xs, color: 'text.disabled' }}>{timeAgo(thread.updated_at, i18n.language, true)}</Typography>
                <Box sx={{ width: 14, flexShrink: 0, display: 'flex', justifyContent: 'center' }}>
                    {thread.status !== 'idle' && <ThreadStateMark status={thread.status} />}
                </Box>
            </Box>
            {tasks.map((task) => (
                <Box key={task.task_id} title={task.description} sx={{ ...ROW_SX, pl: SESSION_INDENT, minHeight: 24, color: 'text.secondary' }}>
                    <Typography noWrap sx={{ flex: 1, minWidth: 0, fontSize: fontSizes.sm, color: 'inherit' }}>{task.description}</Typography>
                    <Box sx={{ width: 14, flexShrink: 0, display: 'flex', justifyContent: 'center' }}><Terminal sx={{ fontSize: 14 }} /></Box>
                </Box>
            ))}
        </>
    );
}

/** What a folded folder says about its sessions: one dot, the most urgent first. */
function Rollup({ folder }: { folder: ProjectSummary }) {
    const { working, waiting, failed } = folder.threads;
    const status = failed > 0 ? 'failed' : waiting > 0 ? 'waiting' : working > 0 ? 'working' : null;
    if (!status) return null;
    return (
        <Box
            aria-hidden
            data-testid="rollup"
            data-status={status}
            sx={(theme) => ({ width: 8, height: 8, mr: '3px', borderRadius: '50%', flexShrink: 0, bgcolor: status === 'working' ? 'text.secondary' : stateColor(status)(theme) })}
        />
    );
}

function TreeNode({ folder, isFolder, active, openSession, to, sessionTo, onNavigate }: {
    folder: ProjectSummary;
    /** A working folder (else a project): only the icon and the name differ. */
    isFolder: boolean;
    /** The page open is this node's. */
    active: boolean;
    openSession: string | null;
    to: string;
    sessionTo: (id: string) => string;
    onNavigate?: () => void;
}) {
    const { t } = useTranslation('projects');
    const label = isFolder ? folderName(folder.path ?? '') : folder.name;
    const path = folder.path ?? '';
    const isOpenFolder = active;
    const key = `nav.folder.${folder.id}`;
    // Remembered per browser; a folder the user is in starts open.
    const [folded, setFolded] = useState(() => {
        const saved = readLocal(key);
        return saved ? saved === 'folded' : !isOpenFolder;
    });
    const [all, setAll] = useState(false);
    useEffect(() => {
        if (isOpenFolder && readLocal(key) == null) setFolded(false);
    }, [isOpenFolder, key]);
    const open = !folded;
    const sessions = sortSessions(useFolderSessions(folder, open));
    const shown = sessions.filter((th, i) => (i < VISIBLE && th.status !== 'resolved') || rank(th) < 2 || th.id === openSession);
    const older = sessions.length - shown.length;
    const toggle = () => {
        writeLocal(key, folded ? 'open' : 'folded');
        setFolded(!folded);
    };
    const selected = isOpenFolder && !openSession;
    return (
        <Box>
            <Box sx={{ ...ROW_SX, minHeight: 32, mt: 0.5, color: selected ? 'text.primary' : 'text.secondary', ...(selected && { backgroundColor: 'action.selected' }) }}>
                <Box component="span" aria-hidden sx={{ width: 16, flexShrink: 0, display: 'flex', justifyContent: 'center', color: 'text.secondary', '& svg': { fontSize: 16 } }}>
                    {isFolder ? <Folder /> : folder.starred ? <Star /> : <Stack />}
                </Box>
                <Box
                    component={NavLink}
                    to={to}
                    onClick={onNavigate}
                    title={isFolder ? path : label}
                    aria-current={selected ? 'page' : undefined}
                    sx={{ ...LABEL_SX, minWidth: 0, color: 'inherit', textDecoration: 'none', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', '&:hover': { color: 'text.primary' } }}
                >
                    {label}
                </Box>
                <IconButton size="small" aria-label={t(folded ? 'sidebar.unfold' : 'sidebar.fold', { name: label })} aria-expanded={open} onClick={toggle} sx={{ p: 0, color: 'text.disabled', '&:hover': { color: 'text.primary' } }}>
                    {folded ? <ChevronRight sx={{ fontSize: 14 }} /> : <ExpandMore sx={{ fontSize: 14 }} />}
                </IconButton>
                <Box sx={{ flex: 1 }} />
                {folded && <Rollup folder={folder} />}
            </Box>
            {open && shown.map((th) => (
                <SessionRow key={th.id} thread={th} to={sessionTo(th.id)} selected={th.id === openSession} onNavigate={onNavigate} />
            ))}
            {open && older > 0 && !all && (
                <ButtonBase onClick={() => setAll(true)} sx={{ ...ROW_SX, pl: SESSION_INDENT, width: 'calc(100% - 16px)', minHeight: 28, justifyContent: 'flex-start', color: 'text.secondary', fontSize: fontSizes.sm }}>
                    {t('sidebar.showOlder', { count: older })}
                </ButtonBase>
            )}
            {open && all && sessions.filter((th) => !shown.includes(th)).map((th) => (
                <SessionRow key={th.id} thread={th} to={sessionTo(th.id)} selected={th.id === openSession} onNavigate={onNavigate} />
            ))}
        </Box>
    );
}

/**
 * Projects and working folders as one tree. A node is an icon (project or
 * folder), a name and a fold caret; its sessions sit beneath it, lined up with
 * the name, with the time and state mark at the end of the row. Folded, the
 * node shows one dot for the most urgent state.
 */
export function NavTree({ items, isFolder, activeId, openSession, openParam, to, onNavigate }: {
    items: ProjectSummary[];
    isFolder: boolean;
    /** Id of the node whose page is open. */
    activeId?: string;
    openSession: string | null;
    /** Query key a session opens under ("session" for folders, "thread" for projects). */
    openParam: string;
    to: (item: ProjectSummary) => string;
    onNavigate?: () => void;
}) {
    return (
        <Box>
            {items.map((f) => {
                const href = to(f);
                return (
                    <TreeNode
                        key={f.id}
                        folder={f}
                        isFolder={isFolder}
                        active={activeId === f.id}
                        openSession={openSession}
                        to={href}
                        sessionTo={(id) => `${href}${href.includes('?') ? '&' : '?'}${openParam}=${encodeURIComponent(id)}`}
                        onNavigate={onNavigate}
                    />
                );
            })}
        </Box>
    );
}
