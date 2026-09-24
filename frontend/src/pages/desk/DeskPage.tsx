import DeskSidebar from '@/components/desk/DeskSidebar';
import NewSessionView from '@/components/desk/NewSessionView';
import SessionView from '@/components/desk/SessionView';
import {isBusyStatus} from '@/components/desk/deskUtils';
import {useNotify} from '@/hooks/useNotify';
import * as deskApi from '@/services/deskApi';
import type {MessageInfo, RecentFolder, SessionInfo} from '@/services/deskApi';
import {Box, useMediaQuery, useTheme} from '@mui/material';
import {useCallback, useEffect, useRef, useState} from 'react';
import {useSearchParams} from 'react-router-dom';
import {useTranslation} from 'react-i18next';

const SESSIONS_POLL_MS = 5000;
const MESSAGES_POLL_MS = 1500;

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
    const [messages, setMessages] = useState<MessageInfo[]>([]);
    const [loading, setLoading] = useState(true);

    const selectedSession = sessions.find((s) => s.id === selectedId) || null;
    const selectedBusy = selectedSession ? isBusyStatus(selectedSession.status) : false;
    // Lets a late messages response for a previously selected session be
    // dropped instead of overwriting the current one's transcript.
    const selectedIdRef = useRef(selectedId);
    useEffect(() => {
        selectedIdRef.current = selectedId;
    }, [selectedId]);

    const loadSessions = useCallback(async () => {
        try {
            setSessions(await deskApi.listSessions());
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.loadFailed', {defaultValue: 'Failed to load sessions'}));
        }
    }, [notify, t]);

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

    useEffect(() => {
        setLoading(true);
        Promise.all([loadSessions(), loadRecentFolders()]).finally(() => setLoading(false));
    }, [loadSessions, loadRecentFolders]);

    // A slow background refresh keeps statuses in the list current even
    // while the user is reading a different session's transcript — scoped
    // to this page only (ux-principles #12), it stops on unmount.
    useEffect(() => {
        const id = setInterval(loadSessions, SESSIONS_POLL_MS);
        return () => clearInterval(id);
    }, [loadSessions]);

    const loadMessages = useCallback(async (sessionId: string) => {
        try {
            const msgs = await deskApi.getMessages(sessionId);
            if (selectedIdRef.current === sessionId) setMessages(msgs);
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.loadFailed', {defaultValue: 'Failed to load messages'}));
        }
    }, [notify, t]);

    // Refreshes only the selected session's own row (a GET by id) rather than
    // the whole list — the fast poll below runs every 1.5s while a turn is
    // busy, so refetching every session on that cadence would be wasted work
    // for the N-1 sessions that aren't the one being watched.
    const refreshSelectedSession = useCallback(async (id: string) => {
        try {
            const updated = await deskApi.getSession(id);
            setSessions((prev) => prev.map((s) => (s.id === updated.id ? updated : s)));
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.loadFailed', {defaultValue: 'Failed to load session'}));
        }
    }, [notify, t]);

    // Load the transcript on selection and once more when a turn ends (the
    // last poll tick's messages can predate the status that stops polling),
    // and poll fast only while a turn is running or an approval waits, so an
    // idle session doesn't keep polling forever.
    useEffect(() => {
        if (!selectedId) {
            setMessages([]);
            return;
        }
        void loadMessages(selectedId);
        if (!selectedBusy) return;
        const id = setInterval(() => {
            void loadMessages(selectedId);
            void refreshSelectedSession(selectedId);
        }, MESSAGES_POLL_MS);
        return () => clearInterval(id);
    }, [selectedId, selectedBusy, loadMessages, refreshSelectedSession]);

    // Resolves false on failure so the composer keeps what the user typed.
    const handleCreate = async (path: string, prompt: string, permissionMode: string, profile: string): Promise<boolean> => {
        try {
            const session = await deskApi.createSession(path, prompt, permissionMode || undefined, profile || undefined);
            // A brand-new session (and possibly a brand-new folder) needs the
            // full lists, unlike the single-session refreshes below.
            await Promise.all([loadSessions(), loadRecentFolders()]);
            setSearchParams({session: session.id});
            return true;
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.startFailed', {defaultValue: 'Failed to start session'}));
            return false;
        }
    };

    const handleSend = async (text: string): Promise<boolean> => {
        if (!selectedId) return false;
        try {
            await deskApi.sendMessage(selectedId, text);
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.sendFailed', {defaultValue: 'Failed to send message'}));
            return false;
        }
        await Promise.all([loadMessages(selectedId), refreshSelectedSession(selectedId)]);
        return true;
    };

    const handleRespond = async (requestId: string, approved: boolean, answer: string) => {
        if (!selectedId) return;
        try {
            await deskApi.respond(selectedId, requestId, approved, answer);
            await loadMessages(selectedId);
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.respondFailed', {defaultValue: 'Failed to respond'}));
        }
    };

    const handleInterrupt = async () => {
        if (!selectedId) return;
        try {
            await deskApi.interrupt(selectedId);
            await Promise.all([loadMessages(selectedId), refreshSelectedSession(selectedId)]);
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.interruptFailed', {defaultValue: 'Failed to interrupt'}));
        }
    };

    const handleArchive = async () => {
        if (!selectedId) return;
        try {
            await deskApi.archive(selectedId);
            await refreshSelectedSession(selectedId);
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.archiveFailed', {defaultValue: 'Failed to archive'}));
        }
    };

    const handleProfileChange = async (profile: string) => {
        if (!selectedId) return;
        try {
            const updated = await deskApi.setProfile(selectedId, profile);
            setSessions((prev) => prev.map((s) => (s.id === updated.id ? updated : s)));
            await loadMessages(selectedId);
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.profileFailed', {defaultValue: 'Failed to change profile'}));
        }
    };

    const handlePermissionModeChange = async (mode: string) => {
        if (!selectedId) return;
        try {
            const updated = await deskApi.setPermissionMode(selectedId, mode);
            setSessions((prev) => prev.map((s) => (s.id === updated.id ? updated : s)));
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.updateFailed', {defaultValue: 'Failed to update permission mode'}));
        }
    };

    const openSession = (id: string) => setSearchParams({session: id});
    const openNew = (folder?: string) => setSearchParams(folder ? {new: '1', folder} : {new: '1'});
    const backToList = () => setSearchParams({});

    // On narrow screens the list and the work area are separate views: the
    // list shows until a session (or a new one) is opened.
    const showList = !isNarrow || (!selectedId && !searchParams.has('new'));
    const showMain = !isNarrow || !showList;

    return (
        <Box
            sx={{
                height: '100%',
                minHeight: 520,
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
                    <DeskSidebar sessions={sessions} selectedId={selectedId} onSelect={openSession} onNew={openNew}/>
                </Box>
            )}
            {showMain && (
                <Box sx={{flex: 1, minWidth: 0}}>
                    {loading ? null : selectedSession ? (
                        <SessionView
                            session={selectedSession}
                            messages={messages}
                            permissionModes={permissionModes}
                            onSend={handleSend}
                            onRespond={handleRespond}
                            onInterrupt={handleInterrupt}
                            onArchive={handleArchive}
                            onPermissionModeChange={handlePermissionModeChange}
                            onProfileChange={handleProfileChange}
                            onBack={isNarrow ? backToList : undefined}
                        />
                    ) : (
                        <NewSessionView
                            // Remount per folder so a folder group's "+" resets the form.
                            key={searchParams.get('folder') ?? ''}
                            initialFolder={searchParams.get('folder') ?? undefined}
                            recentFolders={recentFolders}
                            permissionModes={permissionModes}
                            onCreate={handleCreate}
                        />
                    )}
                </Box>
            )}
        </Box>
    );
};

export default DeskPage;
