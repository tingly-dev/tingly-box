import {PageLayout} from '@/components/PageLayout';
import EmptyState from '@/components/EmptyState';
import UnifiedCard from '@/components/UnifiedCard';
import SessionListPanel from '@/components/managed-agent/SessionListPanel';
import TranscriptPanel from '@/components/managed-agent/TranscriptPanel';
import {isBusyStatus} from '@/components/managed-agent/managedAgentUtils';
import {useNotify} from '@/hooks/useNotify';
import * as managedAgentApi from '@/services/managedAgentApi';
import type {MessageInfo, RecentFolder, SessionInfo} from '@/services/managedAgentApi';
import {Grid} from '@mui/material';
import {useCallback, useEffect, useState} from 'react';
import {useSearchParams} from 'react-router-dom';
import {useTranslation} from 'react-i18next';

const SESSIONS_POLL_MS = 5000;
const MESSAGES_POLL_MS = 1500;

// ManagedAgentPage is the web-side twin of `claude`/`tingly-box cc` run
// locally: pick a folder, start or resume a session, watch the same turn
// structure (thinking / tool calls / approvals) a terminal would show, and
// answer approvals from here instead of a local prompt. It reuses the exact
// backend machinery @cc already drives (remote/session.Manager +
// agentboot.AgentService) — see .design/managed-agent.md.
const ManagedAgentPage = () => {
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
            const [list, folders] = await Promise.all([
                managedAgentApi.listSessions(),
                managedAgentApi.listRecentFolders(),
            ]);
            setSessions(list);
            setRecentFolders(folders);
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('managedAgent.loadFailed', {defaultValue: 'Failed to load sessions'}));
        }
    }, [notify, t]);

    useEffect(() => {
        managedAgentApi.listPermissionModes().then(setPermissionModes).catch(() => {});
    }, []);

    useEffect(() => {
        setLoading(true);
        loadSessions().finally(() => setLoading(false));
    }, [loadSessions]);

    // A slow background refresh keeps statuses in the list current even
    // while the user is reading a different session's transcript — scoped
    // to this page only (ux-principles #12), it stops on unmount.
    useEffect(() => {
        const id = setInterval(loadSessions, SESSIONS_POLL_MS);
        return () => clearInterval(id);
    }, [loadSessions]);

    const loadMessages = useCallback(async (sessionId: string) => {
        try {
            setMessages(await managedAgentApi.getMessages(sessionId));
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('managedAgent.loadFailed', {defaultValue: 'Failed to load messages'}));
        }
    }, [notify, t]);

    useEffect(() => {
        if (!selectedId) {
            setMessages([]);
            return;
        }
        void loadMessages(selectedId);
    }, [selectedId, loadMessages]);

    // Fast polling only while there's actually something moving — a turn
    // running or an approval waiting — so an idle, completed session
    // doesn't keep polling forever.
    useEffect(() => {
        if (!selectedId || !selectedSession || !isBusyStatus(selectedSession.status)) return;
        const id = setInterval(() => {
            void loadMessages(selectedId);
            void loadSessions();
        }, MESSAGES_POLL_MS);
        return () => clearInterval(id);
    }, [selectedId, selectedSession, loadMessages, loadSessions]);

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
            const session = await managedAgentApi.createSession(path, prompt, permissionMode || undefined);
            await loadSessions();
            selectSession(session.id);
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('managedAgent.startFailed', {defaultValue: 'Failed to start session'}));
        } finally {
            setCreating(false);
        }
    };

    const handleSend = async (text: string) => {
        if (!selectedId) return;
        try {
            await managedAgentApi.sendMessage(selectedId, text);
            await Promise.all([loadMessages(selectedId), loadSessions()]);
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('managedAgent.sendFailed', {defaultValue: 'Failed to send message'}));
        }
    };

    const handleRespond = async (requestId: string, approved: boolean, answer: string) => {
        if (!selectedId) return;
        try {
            await managedAgentApi.respond(selectedId, requestId, approved, answer);
            await loadMessages(selectedId);
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('managedAgent.respondFailed', {defaultValue: 'Failed to respond'}));
        }
    };

    const handleInterrupt = async () => {
        if (!selectedId) return;
        try {
            await managedAgentApi.interrupt(selectedId);
            await Promise.all([loadMessages(selectedId), loadSessions()]);
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('managedAgent.interruptFailed', {defaultValue: 'Failed to interrupt'}));
        }
    };

    const handleArchive = async () => {
        if (!selectedId) return;
        try {
            await managedAgentApi.archive(selectedId);
            await loadSessions();
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('managedAgent.archiveFailed', {defaultValue: 'Failed to archive'}));
        }
    };

    const handlePermissionModeChange = async (mode: string) => {
        if (!selectedId) return;
        try {
            const updated = await managedAgentApi.setPermissionMode(selectedId, mode);
            setSessions((prev) => prev.map((s) => (s.id === updated.id ? updated : s)));
        } catch (err) {
            notify.error(err instanceof Error ? err.message : t('managedAgent.updateFailed', {defaultValue: 'Failed to update permission mode'}));
        }
    };

    return (
        <PageLayout
            loading={loading}
            title={t('managedAgent.title', {defaultValue: 'Managed Agent'})}
            subtitle={t('managedAgent.subtitle', {defaultValue: 'Run Claude Code in a folder on this machine, from any browser.'})}
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
                                title={t('managedAgent.noSelection', {defaultValue: 'No session selected'})}
                                description={t('managedAgent.noSelectionDescription', {defaultValue: 'Start a new one on the left, or pick one from the list.'})}
                            />
                        </UnifiedCard>
                    )}
                </Grid>
            </Grid>
        </PageLayout>
    );
};

export default ManagedAgentPage;
