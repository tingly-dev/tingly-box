import {PageLayout} from '@/components/PageLayout';
import EmptyState from '@/components/EmptyState';
import UnifiedCard from '@/components/UnifiedCard';
import SessionListPanel from '@/components/desk/SessionListPanel';
import TranscriptPanel from '@/components/desk/TranscriptPanel';
import {isBusyStatus} from '@/components/desk/deskUtils';
import {useNotify} from '@/hooks/useNotify';
import * as deskApi from '@/services/deskApi';
import type {MessageInfo, RecentFolder, SessionInfo} from '@/services/deskApi';
import {Grid} from '@mui/material';
import {useCallback, useEffect, useState} from 'react';
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

    const [sessions, setSessions] = useState<SessionInfo[]>([]);
    const [recentFolders, setRecentFolders] = useState<RecentFolder[]>([]);
    const [permissionModes, setPermissionModes] = useState<string[]>([]);
    const [messages, setMessages] = useState<MessageInfo[]>([]);
    const [loading, setLoading] = useState(true);
    const [creating, setCreating] = useState(false);

    const selectedSession = sessions.find((s) => s.id === selectedId) || null;

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
            setMessages(await deskApi.getMessages(sessionId));
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.loadFailed', {defaultValue: 'Failed to load messages'}));
        }
    }, [notify, t]);

    useEffect(() => {
        if (!selectedId) {
            setMessages([]);
            return;
        }
        void loadMessages(selectedId);
    }, [selectedId, loadMessages]);

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

    // Fast polling only while there's actually something moving — a turn
    // running or an approval waiting — so an idle, completed session
    // doesn't keep polling forever.
    useEffect(() => {
        if (!selectedId || !selectedSession || !isBusyStatus(selectedSession.status)) return;
        const id = setInterval(() => {
            void loadMessages(selectedId);
            void refreshSelectedSession(selectedId);
        }, MESSAGES_POLL_MS);
        return () => clearInterval(id);
    }, [selectedId, selectedSession, loadMessages, refreshSelectedSession]);

    const selectSession = (id: string) => {
        setSearchParams((prev) => {
            const next = new URLSearchParams(prev);
            next.set('session', id);
            return next;
        });
    };

    const handleCreate = async (path: string, prompt: string, permissionMode: string) => {
        setCreating(true);
        try {
            const session = await deskApi.createSession(path, prompt, permissionMode || undefined);
            // A brand-new session (and possibly a brand-new folder) needs the
            // full lists, unlike the single-session refreshes below.
            await Promise.all([loadSessions(), loadRecentFolders()]);
            selectSession(session.id);
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.startFailed', {defaultValue: 'Failed to start session'}));
        } finally {
            setCreating(false);
        }
    };

    const handleSend = async (text: string) => {
        if (!selectedId) return;
        try {
            await deskApi.sendMessage(selectedId, text);
            await Promise.all([loadMessages(selectedId), refreshSelectedSession(selectedId)]);
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.sendFailed', {defaultValue: 'Failed to send message'}));
        }
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

    const handlePermissionModeChange = async (mode: string) => {
        if (!selectedId) return;
        try {
            const updated = await deskApi.setPermissionMode(selectedId, mode);
            setSessions((prev) => prev.map((s) => (s.id === updated.id ? updated : s)));
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('desk.updateFailed', {defaultValue: 'Failed to update permission mode'}));
        }
    };

    return (
        <PageLayout
            loading={loading}
            title={t('desk.title', {defaultValue: 'Desk'})}
            subtitle={t('desk.subtitle', {defaultValue: 'Run Claude Code in a folder on this machine, from any browser.'})}
        >
            <Grid container spacing={2}>
                <Grid size={{xs: 12, md: 4}}>
                    <SessionListPanel
                        sessions={sessions}
                        recentFolders={recentFolders}
                        permissionModes={permissionModes}
                        selectedId={selectedId}
                        onSelect={selectSession}
                        onCreate={handleCreate}
                        creating={creating}
                    />
                </Grid>
                <Grid size={{xs: 12, md: 8}}>
                    {selectedSession ? (
                        <TranscriptPanel
                            session={selectedSession}
                            messages={messages}
                            permissionModes={permissionModes}
                            onSend={handleSend}
                            onRespond={handleRespond}
                            onInterrupt={handleInterrupt}
                            onArchive={handleArchive}
                            onPermissionModeChange={handlePermissionModeChange}
                        />
                    ) : (
                        <UnifiedCard size="full">
                            <EmptyState
                                title={t('desk.noSelection', {defaultValue: 'No session selected'})}
                                description={t('desk.noSelectionDescription', {defaultValue: 'Start a new one on the left, or pick one from the list.'})}
                            />
                        </UnifiedCard>
                    )}
                </Grid>
            </Grid>
        </PageLayout>
    );
};

export default DeskPage;
