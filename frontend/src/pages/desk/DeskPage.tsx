import DeskSidebar from '@/components/desk/DeskSidebar';
import AddProjectDialog from '@/components/desk/AddProjectDialog';
import NewSessionView from '@/components/desk/NewSessionView';
import SessionView from '@/components/desk/SessionView';
import {folderName, isBusyStatus} from '@/components/desk/deskUtils';
import {requestNotifications, useDeskAttention} from '@/components/desk/useDeskAttention';
import {useDeskDrafts} from '@/components/desk/useDeskDrafts';
import {useDeskQueues} from '@/components/desk/useDeskQueues';
import {useDeskPoll} from '@/components/desk/useDeskPoll';
import {useDeskProjects} from '@/components/desk/useDeskProjects';
import {useNotify} from '@/hooks/useNotify';
import * as deskApi from '@/services/deskApi';
import type {MessageInfo, RecentFolder, SessionInfo} from '@/services/deskApi';
import {Alert, Box, Button, CircularProgress, Stack, useMediaQuery, useTheme} from '@mui/material';
import {useCallback, useEffect, useLayoutEffect, useRef, useState} from 'react';
import {useSearchParams} from 'react-router-dom';
import {useTranslation} from 'react-i18next';

const SESSIONS_POLL_MS = 5000;
const MESSAGES_POLL_MS = 1500;
const READ_TIMEOUT_MS = 15000;

// DeskPage is the web-side twin of `claude`/`tingly-box cc` run
// locally: pick a folder, start or resume a session, watch the same turn
// structure (thinking / tool calls / approvals) a terminal would show, and
// answer approvals from here instead of a local prompt. It reuses the exact
// backend machinery @cc already drives (remote/session.Manager +
// agentboot.AgentService) — see .design/desk.md.
const DeskPage = () => {
    const {t} = useTranslation();
    const notify = useNotify();
    const [searchParams, setSearchParams] = useSearchParams();
    const selectedId = searchParams.get('session');
    const theme = useTheme();
    const isNarrow = useMediaQuery(theme.breakpoints.down('md'));

    const [sessions, setSessions] = useState<SessionInfo[]>([]);
    const [recentFolders, setRecentFolders] = useState<RecentFolder[]>([]);
    const [permissionModes, setPermissionModes] = useState<string[]>([]);
    const {projects, addProject, removeProject} = useDeskProjects();
    const [addingProject, setAddingProject] = useState(false);
    const [pendingProject, setPendingProject] = useState<string | null>(null);
    // Keep the modal over the previous form until the router commits the
    // selected directory, so fast typing reaches the new project's draft.
    useLayoutEffect(() => {
        if (pendingProject && !selectedId && searchParams.has('new') && searchParams.get('folder') === pendingProject) {
            setAddingProject(false);
            setPendingProject(null);
        }
    }, [pendingProject, selectedId, searchParams]);
    const projectFolders = [...projects.filter((path) => !recentFolders.some((f) => f.path === path)).map((path) => ({path, name: folderName(path), last_used_at: ''})), ...recentFolders];
    const [transcript, setTranscript] = useState<{id: string; messages: MessageInfo[]} | null>(null);
    const messages = transcript?.id === selectedId ? transcript.messages : [];
    const [sessionsError, setSessionsError] = useState<string | null>(null);
    const [transcriptError, setTranscriptError] = useState<{id: string; message: string} | null>(null);
    const [sessionError, setSessionError] = useState<{id: string; message: string} | null>(null);
    const [loading, setLoading] = useState(true);
    // Per session: follow-ups queued behind a running turn, and the unsent
    // text in the composer (kept when switching between sessions).
    const [queues, setQueues, queuesRef] = useDeskQueues();
    const [drafts, setDrafts] = useDeskDrafts('desk.sessionDrafts');

    const selectedSession = sessions.find((s) => s.id === selectedId) || null;
    const selectedBusy = selectedSession ? isBusyStatus(selectedSession.status) : false;
    // Background tasks report between turns (progress, and a turn Claude
    // starts itself when one finishes), so a session running any is watched
    // as closely as one running a turn.
    const selectedActive = selectedBusy || (selectedSession?.background_tasks?.length ?? 0) > 0;
    // Lets a late messages response for a previously selected session be
    // dropped instead of overwriting the current one's transcript.
    const selectedIdRef = useRef(selectedId);
    const listRequest = useRef(0);
    const messageRequest = useRef(0);
    useLayoutEffect(() => {
        selectedIdRef.current = selectedId;
        ++messageRequest.current;
    }, [selectedId]);
    const rowRequests = useRef(new Map<string, number>());
    const mounted = useRef(true);
    useEffect(() => {
        mounted.current = true;
        return () => { mounted.current = false; };
    }, []);

    const replaceSession = (updated: SessionInfo) => {
        rowRequests.current.set(updated.id, (rowRequests.current.get(updated.id) ?? 0) + 1);
        setSessions((prev) => prev.map((s) => s.id === updated.id ? updated : s));
    };

    const loadSessions = useCallback(async (signal: AbortSignal = AbortSignal.timeout(READ_TIMEOUT_MS)) => {
        const request = ++listRequest.current;
        const rows = new Map(rowRequests.current);
        try {
            const updated = await deskApi.listSessions(undefined, signal);
            if (!mounted.current || signal?.aborted || request !== listRequest.current) return;
            const preserve = new Set<string>();
            for (const s of updated) {
                if (rows.get(s.id) !== rowRequests.current.get(s.id)) preserve.add(s.id);
                else rowRequests.current.set(s.id, (rowRequests.current.get(s.id) ?? 0) + 1);
            }
            setSessions((prev) => updated.map((s) =>
                preserve.has(s.id) ? prev.find((p) => p.id === s.id) ?? s : s));
            setSessionsError(null);
        } catch (err) {
            if (mounted.current && signal?.reason?.name !== 'AbortError' && request === listRequest.current) {
                setSessionsError(signal?.reason?.name === 'TimeoutError' ? t('desk.refreshTimeout', {defaultValue: 'The server took too long to respond.'})
                    : err instanceof Error ? err.message : t('desk.loadFailed', {defaultValue: 'Failed to load sessions'}));
            }
        } finally {
            if (mounted.current && request === listRequest.current) setLoading(false);
        }
    }, [t]);

    // Recent folders only change when a session starts in a new one
    // (handleCreate re-fetches it directly), so this never needs to be part
    // of the session-list poll.
    const loadRecentFolders = useCallback(async () => {
        try {
            setRecentFolders(await deskApi.listRecentFolders());
        } catch {
            // Non-critical: the composer still works with an empty recent list.
        }
    }, []);

    useEffect(() => {
        deskApi.listPermissionModes().then(setPermissionModes).catch(() => {});
    }, []);

    useEffect(() => { void loadRecentFolders(); }, [loadRecentFolders]);

    // A slow background refresh keeps statuses in the list current even
    // while the user is reading a different session's transcript — scoped
    // to this page only (ux-principles #12), it stops on unmount.
    useDeskPoll(loadSessions, SESSIONS_POLL_MS);

    const loadMessages = useCallback(async (sessionId: string, signal: AbortSignal = AbortSignal.timeout(READ_TIMEOUT_MS)) => {
        if (selectedIdRef.current !== sessionId) return;
        const request = ++messageRequest.current;
        try {
            const msgs = await deskApi.getMessages(sessionId, signal);
            if (mounted.current && !signal?.aborted && selectedIdRef.current === sessionId && request === messageRequest.current) {
                setTranscript({id: sessionId, messages: msgs});
                setTranscriptError(null);
            }
        } catch (err) {
            if (mounted.current && signal?.reason?.name !== 'AbortError' && selectedIdRef.current === sessionId && request === messageRequest.current) {
                setTranscriptError({id: sessionId, message: signal?.reason?.name === 'TimeoutError' ? t('desk.refreshTimeout', {defaultValue: 'The server took too long to respond.'})
                    : err instanceof Error ? err.message : t('desk.loadFailed', {defaultValue: 'Failed to load messages'})});
            }
        }
    }, [t]);

    // Refreshes only the selected session's own row (a GET by id) rather than
    // the whole list — the fast poll below runs every 1.5s while a turn is
    // busy, so refetching every session on that cadence would be wasted work
    // for the N-1 sessions that aren't the one being watched.
    const refreshSelectedSession = useCallback(async (id: string, signal: AbortSignal = AbortSignal.timeout(READ_TIMEOUT_MS)) => {
        const request = (rowRequests.current.get(id) ?? 0) + 1;
        rowRequests.current.set(id, request);
        try {
            const updated = await deskApi.getSession(id, signal);
            if (!mounted.current || signal?.aborted || rowRequests.current.get(id) !== request) return;
            rowRequests.current.set(id, request + 1);
            setSessions((prev) => prev.map((s) => (s.id === updated.id ? updated : s)));
            if (selectedIdRef.current === id) setSessionError(null);
        } catch (err) {
            if (mounted.current && signal?.reason?.name !== 'AbortError' && selectedIdRef.current === id && rowRequests.current.get(id) === request) {
                setSessionError({id, message: signal?.reason?.name === 'TimeoutError' ? t('desk.refreshTimeout', {defaultValue: 'The server took too long to respond.'})
                    : err instanceof Error ? err.message : t('desk.loadFailed', {defaultValue: 'Failed to load session'})});
            }
        }
    }, [t]);

    // Refresh immediately on selection and when a turn ends. Idle sessions
    // still refresh slowly: a failed initial load, a server restart or a
    // terminal handoff can change them without an in-page running turn.
    const refreshTranscript = useCallback(async (signal?: AbortSignal) => {
        if (!selectedId) return;
        await Promise.all([loadMessages(selectedId, signal), refreshSelectedSession(selectedId, signal)]);
    }, [selectedId, loadMessages, refreshSelectedSession]);
    useDeskPoll(refreshTranscript, selectedActive ? MESSAGES_POLL_MS : SESSIONS_POLL_MS, Boolean(selectedId), selectedId);

    // Resolves false on failure so the composer keeps what the user typed.
    const handleCreate = async (path: string, prompt: string, permissionMode: string, profile: string, model: string): Promise<boolean> => {
        requestNotifications();
        try {
            const session = await deskApi.createSession(path, prompt, permissionMode || undefined, profile || undefined, model || undefined);
            ++listRequest.current; // Invalidate a pre-creation list response.
            rowRequests.current.set(session.id, 1);
            setSessions((prev) => [session, ...prev.filter((s) => s.id !== session.id)]);
            // A brand-new session (and possibly a brand-new folder) needs the
            // full lists, unlike the single-session refreshes below.
            setSearchParams({session: session.id});
            void Promise.all([loadSessions(), loadRecentFolders()]);
            return true;
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.startFailed', {defaultValue: 'Failed to start session'}));
            return false;
        }
    };

    // send posts a message to any session (not only the one on screen: a
    // queue drains wherever it is).
    const send = useCallback(async (id: string, text: string): Promise<boolean> => {
        try {
            await deskApi.sendMessage(id, text);
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.sendFailed', {defaultValue: 'Failed to send message'}));
            return false;
        }
        // Acceptance releases the composer immediately. Optimistic busy state
        // prevents a second draft bypassing the queue while reads catch up.
        rowRequests.current.set(id, (rowRequests.current.get(id) ?? 0) + 1);
        setSessions((prev) => prev.map((session) => session.id === id ? {...session, status: 'pending', awaiting_input: false} : session));
        if (selectedIdRef.current === id) {
            void Promise.all([loadMessages(id), refreshSelectedSession(id)]);
        } else {
            void loadSessions();
        }
        return true;
    }, [notify, t, loadMessages, refreshSelectedSession, loadSessions]);

    // While a turn runs, a message waits in the queue instead of being
    // refused; the drain below sends it when the turn ends.
    const handleSend = async (text: string): Promise<boolean> => {
        if (!selectedId) return false;
        requestNotifications();
        if (selectedBusy || queuesRef.current[selectedId]?.items.length) {
            setQueues((q) => ({...q, [selectedId]: {...q[selectedId], items: [...(q[selectedId]?.items ?? []), text]}}));
            return true;
        }
        return send(selectedId, text);
    };

    // Serialize sends and persist the pending prefix through the POST. Polls
    // may append new items, but only the accepted prefix is removed.
    const flushQueue = useCallback(async (session: SessionInfo, manual = false) => {
        const id = session.id;
        const queue = queuesRef.current[id];
        if (!queue?.items.length || queue.inFlight || isBusyStatus(session.status) || session.status === 'closed' || (queue.held && !manual)) return;
        const items = queue.items;
        setQueues((q) => ({...q, [id]: {...q[id], held: undefined, inFlight: true}}));
        const ok = await send(id, items.join('\n\n'));
        setQueues((q) => ({...q, [id]: {
            items: ok && items.every((text, index) => q[id]?.items[index] === text) ? q[id].items.slice(items.length) : q[id]?.items ?? [],
            inFlight: false,
            held: ok ? q[id]?.held : 'failed',
        }}));
    }, [send, setQueues, queuesRef]);

    useEffect(() => {
        for (const session of sessions) {
            const queue = queues[session.id];
            if (!queue?.items.length || queue.inFlight || queue.held) continue;
            if (session.status === 'completed') void flushQueue(session);
            else if (session.status === 'failed' || session.status === 'closed') {
                setQueues((q) => ({...q, [session.id]: {...q[session.id], held: 'failed'}}));
            }
        }
    }, [sessions, queues, flushQueue, setQueues]);

    const unqueue = (index: number) => {
        if (!selectedId) return;
        const queue = queuesRef.current[selectedId];
        if (queue?.inFlight) return;
        const item = queue?.items[index];
        if (item === undefined) return;
        // Save the destination first so a reload cannot lose the text.
        setDrafts((d) => ({...d, [selectedId]: d[selectedId] ? `${item}\n\n${d[selectedId]}` : item}));
        setQueues((q) => ({...q, [selectedId]: {...q[selectedId], items: q[selectedId].items.filter((_, i) => i !== index)}}));
    };

    const handleRespond = async (requestId: string, approved: boolean, answer: string): Promise<boolean> => {
        if (!selectedId) return false;
        try {
            await deskApi.respond(selectedId, requestId, approved, answer);
            void loadMessages(selectedId);
            return true;
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.respondFailed', {defaultValue: 'Failed to respond'}));
            return false;
        }
    };

    const handleInterrupt = async () => {
        if (!selectedId) return;
        // Stopping means changing course, so queued follow-ups go back into
        // the input rather than out as the next turn (as the terminal does).
        const queue = queuesRef.current[selectedId];
        if (queue?.items.length) {
            setQueues((q) => ({...q, [selectedId]: {...q[selectedId], held: 'paused'}}));
            if (!queue.inFlight) {
                setDrafts((d) => ({...d, [selectedId]: [...queue.items, d[selectedId] ?? ''].filter(Boolean).join('\n\n')}));
                setQueues((q) => ({...q, [selectedId]: {items: []}}));
            }
        }
        try {
            await deskApi.interrupt(selectedId);
            void Promise.all([loadMessages(selectedId), refreshSelectedSession(selectedId)]);
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.interruptFailed', {defaultValue: 'Failed to interrupt'}));
        }
    };

    const handleArchive = async (): Promise<boolean> => {
        if (!selectedId) return false;
        setQueues((q) => ({...q, [selectedId]: {...q[selectedId], items: q[selectedId]?.items ?? [], held: 'paused'}}));
        try {
            replaceSession(await deskApi.archive(selectedId));
            return true;
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.archiveFailed', {defaultValue: 'Failed to archive'}));
            return false;
        }
    };

    const handleHandoff = async (): Promise<string | null> => {
        if (!selectedId) return null;
        setQueues((q) => ({...q, [selectedId]: {...q[selectedId], items: q[selectedId]?.items ?? [], held: 'paused'}}));
        try {
            return await deskApi.handoff(selectedId);
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.handoffFailed', {defaultValue: 'Failed to hand off the session'}));
            return null;
        }
    };

    // Launch settings apply from the next message, which restarts Claude's
    // process — and ends the background tasks it runs. Say so when it will.
    const warnRestart = () => {
        const n = selectedSession?.background_tasks?.length ?? 0;
        if (n > 0) {
            notify.info(t('desk.restartStopsTasks', {
                defaultValue: 'Applies from your next message, which restarts Claude and stops its {{count}} background task(s).',
                count: n,
            }));
        }
    };

    const handleProfileChange = async (profile: string) => {
        if (!selectedId) return;
        try {
            const updated = await deskApi.setProfile(selectedId, profile);
            replaceSession(updated);
            warnRestart();
            void loadMessages(selectedId);
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.profileFailed', {defaultValue: 'Failed to change profile'}));
        }
    };

    const handleModelChange = async (model: string) => {
        if (!selectedId) return;
        try {
            const updated = await deskApi.setModel(selectedId, model);
            replaceSession(updated);
            warnRestart();
            void loadMessages(selectedId);
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.modelFailed', {defaultValue: 'Failed to change model'}));
        }
    };

    const handlePermissionModeChange = async (mode: string) => {
        if (!selectedId) return;
        try {
            const updated = await deskApi.setPermissionMode(selectedId, mode);
            replaceSession(updated);
            warnRestart();
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.updateFailed', {defaultValue: 'Failed to update permission mode'}));
        }
    };

    const openSession = useCallback((id: string) => setSearchParams({session: id}), [setSearchParams]);
    const unseen = useDeskAttention(sessions, selectedId, openSession);
    const openNew = (folder?: string) => setSearchParams(folder ? {new: '1', folder} : {new: '1'});
    const backToList = () => setSearchParams({});

    // On narrow screens the list and the work area are separate views: the
    // list shows until a session (or a new one) is opened.
    const showList = !isNarrow || (!selectedId && !searchParams.has('new'));
    const showMain = !isNarrow || !showList;
    const refreshError = sessionsError || (transcriptError?.id === selectedId ? transcriptError.message : null)
        || (sessionError?.id === selectedId ? sessionError.message : null);

    return (
        <Box
            sx={{
                height: '100%',
                minHeight: 0,
                display: 'flex',
                border: 1,
                borderColor: 'divider',
                borderRadius: 2,
                overflow: 'hidden',
                bgcolor: 'background.paper',
            }}
        >
            {showList && (
                <Box sx={{width: isNarrow ? '100%' : 280, flexShrink: 0, borderRight: isNarrow ? 0 : 1, borderColor: 'divider', bgcolor: 'background.default'}}>
                    <DeskSidebar sessions={sessions} selectedId={selectedId} unseen={unseen} onSelect={openSession} onNew={openNew}
                        projects={projects} selectedProject={searchParams.get('folder') ?? undefined} onAddProject={() => setAddingProject(true)}
                        onRemoveProject={(path) => {
                            if (!removeProject(path)) notify.error(t('desk.projectSaveFailed', {defaultValue: 'Could not save the project in this browser. Please try again.'}));
                            else if (searchParams.get('folder') === path) setSearchParams({});
                        }}/>
                </Box>
            )}
            {showMain && (
                <Box sx={{flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column'}}>
                    {refreshError && (
                        <Alert severity="warning" action={(
                            <Button color="inherit" size="small" onClick={() => void Promise.all([loadSessions(), refreshTranscript()])}>
                                {t('desk.retry', {defaultValue: 'Retry'})}
                            </Button>
                        )}>
                            {t('desk.refreshFailed', {defaultValue: 'Could not refresh Desk. Your last loaded data and drafts are kept. Reconnecting automatically.'})}
                            <Box component="span" sx={{display: 'block', fontSize: '0.8rem'}}>{refreshError}</Box>
                        </Alert>
                    )}
                    <Box sx={{flex: 1, minHeight: 0}}>
                        {loading ? (
                            <Stack sx={{height: '100%', alignItems: 'center', justifyContent: 'center'}}><CircularProgress size={24}/></Stack>
                        ) : selectedSession ? (
                            <SessionView
                                key={selectedSession.id}
                                session={selectedSession}
                                messages={messages}
                                permissionModes={permissionModes}
                                onSend={handleSend}
                                onRespond={handleRespond}
                                onInterrupt={handleInterrupt}
                                onArchive={handleArchive}
                                onPermissionModeChange={handlePermissionModeChange}
                                onProfileChange={handleProfileChange}
                                onModelChange={handleModelChange}
                                queued={queues[selectedSession.id]?.items ?? []}
                                queueHeld={queues[selectedSession.id]?.held}
                                queueSending={Boolean(queues[selectedSession.id]?.inFlight)}
                                messagesLoading={transcript?.id !== selectedSession.id && transcriptError?.id !== selectedSession.id}
                                onUnqueue={unqueue}
                                onSendQueuedNow={() => void flushQueue(selectedSession, true)}
                                draft={drafts[selectedSession.id] ?? ''}
                                onDraftChange={(text) => setDrafts((d) => ({...d, [selectedSession.id]: text}))}
                                onDraftAccepted={(submitted) => setDrafts((d) => d[selectedSession.id] === submitted ? {...d, [selectedSession.id]: ''} : d)}
                                onHandoff={handleHandoff}
                                onRefresh={() => void Promise.all([loadMessages(selectedSession.id), refreshSelectedSession(selectedSession.id)])}
                                onBack={isNarrow ? backToList : undefined}
                            />
                        ) : selectedId ? (
                            <Alert severity="info" sx={{m: 2}} action={<Button color="inherit" onClick={() => openNew()}>{t('desk.newSession', {defaultValue: 'New session'})}</Button>}>
                                {t('desk.sessionUnavailable', {defaultValue: 'This session is unavailable. Retry refreshing or start a new session.'})}
                            </Alert>
                        ) : sessionsError && sessions.length === 0 ? null : (
                            <NewSessionView
                                // Remount per folder so a folder group's "+" resets the form.
                                key={searchParams.get('folder') ?? ''}
                                initialFolder={searchParams.get('folder') ?? undefined}
                                recentFolders={projectFolders}
                                permissionModes={permissionModes}
                                onCreate={handleCreate}
                                onAddProject={() => setAddingProject(true)}
                                onBack={isNarrow ? backToList : undefined}
                            />
                        )}
                    </Box>
                </Box>
            )}
            {addingProject && <AddProjectDialog onClose={() => {setAddingProject(false); setPendingProject(null);}} onAdd={(path) => {
                if (!addProject(path)) return false;
                setPendingProject(path);
                openNew(path);
                return true;
            }}/>}
        </Box>
    );
};

export default DeskPage;
