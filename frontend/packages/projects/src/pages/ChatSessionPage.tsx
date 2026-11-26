import { useCallback, useEffect, useRef, useState } from 'react';
import { Box, Button, IconButton, Skeleton, Typography } from '@mui/material';
import { useNavigate, useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useDocumentTitle } from '@tingly/ui';
import { api, unwrap, type Thread, type ThreadEvent } from '../client';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { ChatNotices } from '../components/chat/ChatNotices';
import { ModelPicker } from '../components/ModelPicker';
import { ThreadPanel } from '../components/project/ThreadPanel';
import { WorkspaceLayout, useWorkspacePane } from '../components/workspace/WorkspaceLayout';
import { ArrowBack, Delete } from '../components/icons';
import { useProjectEvents, type ProjectEvent } from '../hooks/useProjectEvents';
import { useTranscript } from '../hooks/useTranscript';
import { useNotify } from '../hooks/useNotify';
import { useProjectsHost } from '../host/ProjectsHost';

type ChatDetail = { thread: Thread; sandboxed: boolean };

/** One read-only chat: the ordinary ThreadPanel with the chat profile's notices and no project actions. */
export default function ChatSessionPage() {
    const { threadId = '' } = useParams();
    const { t } = useTranslation('projects');
    const navigate = useNavigate();
    const notify = useNotify();
    const { paths, embedded } = useProjectsHost();
    const pane = useWorkspacePane('chat.');
    const [chat, setChat] = useState<ChatDetail | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [confirmDelete, setConfirmDelete] = useState(false);
    const transcript = useTranscript(threadId);
    const req = useRef(0);

    const load = useCallback(async () => {
        const n = ++req.current;
        try {
            const d = await unwrap(api.GET('/api/v1/chats/{id}', { params: { path: { id: threadId } } }));
            if (n !== req.current) return;
            setChat({ thread: d.thread, sandboxed: d.sandboxed });
            setError(null);
        } catch (e) {
            if (n === req.current) setError((e as Error).message);
        }
    }, [threadId]);

    useEffect(() => {
        setChat(null);
        if (threadId) void load();
    }, [threadId, load]);

    // Live entries come on the chat project's stream; the thread's status moves the panel's running state.
    const onEvent = useRef<(ev: ProjectEvent) => void>(() => undefined);
    useEffect(() => {
        onEvent.current = (ev: ProjectEvent) => {
            if (ev.type === 'thread') {
                const th = ev.data as Thread;
                if (th.id === threadId) setChat((c) => (c ? { ...c, thread: th } : c));
            } else if (ev.type === 'thread_event') {
                const e = ev.data as ThreadEvent;
                if (e.thread_id === threadId) transcript.apply(e);
            }
        };
    });
    const stream = useCallback((ev: ProjectEvent) => onEvent.current(ev), []);
    useProjectEvents(chat?.thread.project_id ?? '', stream, () => { void load(); transcript.catchUp().catch(() => undefined); });

    const title = chat?.thread.title || t('readonlyChats.untitled');
    useDocumentTitle([title]);

    const goList = () => navigate(paths.chats ?? '/workspace/chats');

    const remove = async () => {
        try {
            await unwrap(api.DELETE('/api/v1/chats/{id}', { params: { path: { id: threadId } } }));
            notify(t('readonlyChats.deleted'), 'success');
            goList();
        } catch (e) {
            notify((e as Error).message, 'error');
            setConfirmDelete(false);
        }
    };

    const setModel = async (model: string) => {
        try {
            await unwrap(api.PUT('/api/v1/threads/{id}/model', { params: { path: { id: threadId } }, body: { model } }));
            void load();
        } catch (e) {
            notify((e as Error).message, 'error');
        }
    };

    const actions = (
        <>
            {chat && (
                <ModelPicker source="projects" value={chat.thread.model ?? ''} onChange={(m) => void setModel(m)} disabled={chat.thread.status === 'working'} />
            )}
            <IconButton size="small" aria-label={t('readonlyChats.delete')} onClick={() => setConfirmDelete(true)} sx={{ color: 'text.secondary' }}>
                <Delete fontSize="small" />
            </IconButton>
        </>
    );

    return (
        <>
            <WorkspaceLayout
                pane={pane}
                hidePaneToggle
                headerInsetXs={embedded ? 2 : 6}
                title={<Typography variant="h5" noWrap>{t('readonlyChats.title')}</Typography>}
                actions={actions}
                paneLabel={t('readonlyChats.title')}
                notice={(
                    <>
                        {chat && <Box sx={{ px: 2, py: 1, borderBottom: 1, borderColor: 'divider' }}><ChatNotices sandboxed={chat.sandboxed} /></Box>}
                        {error && <Box sx={{ px: 2, py: 1 }}><Typography color="error" variant="body2">{error}</Typography></Box>}
                    </>
                )}
                main={chat ? (
                    <ThreadPanel
                        key={threadId}
                        projectId={chat.thread.project_id}
                        chat
                        threadId={threadId}
                        thread={chat.thread}
                        events={transcript.events}
                        more={transcript.more}
                        onLoadEarlier={transcript.loadEarlier}
                        subagents={transcript.subagents}
                        running={chat.thread.status === 'working'}
                        disabled={false}
                        onBack={goList}
                        backLabel={t('readonlyChats.title')}
                        onRefresh={() => { void load(); transcript.catchUp().catch(() => undefined); }}
                        permissionLabel={t('readonlyChats.readOnly')}
                        belowComposer={<ChatNotices sandboxed={chat.sandboxed} compact />}
                    />
                ) : (
                    <Box sx={{ p: 3 }}>
                        {error ? (
                            <Button onClick={goList} startIcon={<ArrowBack />}>{t('readonlyChats.title')}</Button>
                        ) : (
                            <><Skeleton height={40} /><Skeleton height={80} /><Skeleton height={120} /></>
                        )}
                    </Box>
                )}
            />
            <ConfirmDialog
                open={confirmDelete}
                title={t('readonlyChats.deleteTitle')}
                message={t('readonlyChats.deleteMessage', { title })}
                confirmLabel={t('readonlyChats.delete')}
                danger
                onClose={() => setConfirmDelete(false)}
                onConfirm={() => void remove()}
            />
        </>
    );
}
