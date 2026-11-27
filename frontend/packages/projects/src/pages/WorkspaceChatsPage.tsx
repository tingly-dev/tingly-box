import { useRef, useState } from 'react';
import { Box, Button, CircularProgress, IconButton, Skeleton, TextField, Tooltip, Typography } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useDocumentTitle, EmptyState, PageHeader } from '@tingly/ui';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import { api, apiFetch, unwrap, type Thread } from '../client';
import { ChatNotices } from '../components/chat/ChatNotices';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { Add, Attach, ChatBubble, Delete } from '../components/icons';
import { useChatList } from '../hooks/useChatList';
import { useNotify } from '../hooks/useNotify';
import { useProjectsHost } from '../host/ProjectsHost';
import { timeAgo } from '../utils/format';

/** Create a chat with its first message and (optional) files, in one multipart request. */
async function createChat(content: string, files: File[]): Promise<Thread> {
    const form = new FormData();
    form.append('content', content);
    for (const f of files) form.append('files', f, f.name);
    const res = await apiFetch('/api/v1/chats', { method: 'POST', body: form });
    const body = (await res.json().catch(() => ({}))) as { error?: string } & Partial<Thread>;
    if (!res.ok) throw new Error(body.error ?? res.statusText);
    return body as Thread;
}

function NewChatBox({ onCreated }: { onCreated: (t: Thread) => void }) {
    const { t } = useTranslation('projects');
    const [text, setText] = useState('');
    const [files, setFiles] = useState<File[]>([]);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const input = useRef<HTMLInputElement>(null);

    const submit = async () => {
        if (busy || (!text.trim() && files.length === 0)) return;
        setBusy(true);
        setError('');
        try {
            const created = await createChat(text.trim(), files);
            setText('');
            setFiles([]);
            onCreated(created);
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusy(false);
        }
    };

    return (
        <Box sx={{ border: 1, borderColor: 'divider', borderRadius: radius.card, p: 1.5, mb: 3, bgcolor: 'background.paper' }}>
            <TextField
                fullWidth
                multiline
                minRows={2}
                maxRows={8}
                autoFocus
                placeholder={t('readonlyChats.newPlaceholder')}
                value={text}
                onChange={(e) => setText(e.target.value)}
                onKeyDown={(e) => {
                    if (e.key === 'Enter' && !e.shiftKey) {
                        e.preventDefault();
                        void submit();
                    }
                }}
                sx={{ '& fieldset': { border: 0 } }}
            />
            {files.length > 0 && (
                <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5, px: 1 }}>
                    {files.map((f, i) => (
                        <Button key={`${f.name}-${i}`} size="small" onClick={() => setFiles(files.filter((_, j) => j !== i))} title={t('readonlyChats.removeFile')}>
                            {f.name}
                        </Button>
                    ))}
                </Box>
            )}
            {error && <Typography variant="caption" color="error" role="alert" sx={{ display: 'block', px: 1, pt: 0.5 }}>{error}</Typography>}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, pt: 1, px: 1 }}>
                <input
                    ref={input}
                    type="file"
                    multiple
                    hidden
                    accept="text/*,application/json,application/xml,application/yaml,image/png,image/jpeg,image/gif,image/webp"
                    onChange={(e) => {
                        setFiles([...files, ...Array.from(e.target.files ?? [])]);
                        if (input.current) input.current.value = '';
                    }}
                />
                <Tooltip title={t('readonlyChats.attach')}>
                    <IconButton size="small" aria-label={t('readonlyChats.attach')} onClick={() => input.current?.click()} sx={{ color: 'text.secondary' }}>
                        <Attach fontSize="small" />
                    </IconButton>
                </Tooltip>
                <Typography variant="caption" color="text.secondary" sx={{ flex: 1 }}>{t('readonlyChats.filesHint')}</Typography>
                <Button variant="contained" size="small" disabled={busy || (!text.trim() && files.length === 0)} onClick={() => void submit()} startIcon={busy ? <CircularProgress size={14} /> : <Add />}>
                    {t('readonlyChats.start')}
                </Button>
            </Box>
        </Box>
    );
}

export default function WorkspaceChatsPage() {
    const { t, i18n } = useTranslation('projects');
    const navigate = useNavigate();
    const notify = useNotify();
    const { paths } = useProjectsHost();
    const { list, reload } = useChatList(!!paths.chats);
    const [deleting, setDeleting] = useState<Thread | null>(null);
    useDocumentTitle([t('readonlyChats.title')]);

    const chats = list?.chats ?? null;
    const sandboxed = list?.sandboxed ?? false;
    const open = (t: Thread) => paths.chat && navigate(paths.chat(t.id));

    const remove = async () => {
        if (!deleting) return;
        try {
            await unwrap(api.DELETE('/api/v1/chats/{id}', { params: { path: { id: deleting.id } } }));
            notify(t('readonlyChats.deleted'), 'success');
            setDeleting(null);
            void reload();
        } catch (e) {
            notify((e as Error).message, 'error');
        }
    };

    return (
        <Box sx={{ flex: 1, overflowY: 'auto' }}>
            <Box sx={{ maxWidth: 860, mx: 'auto', px: { xs: 2, md: 4 }, pt: { xs: 7, md: 5 }, pb: 6 }}>
                <PageHeader
                    title={t('readonlyChats.title')}
                    subtitle={t('readonlyChats.intro')}
                    sx={{ mb: 2 }}
                />
                <Box sx={{ mb: 2 }}><ChatNotices sandboxed={sandboxed} /></Box>
                <NewChatBox onCreated={open} />
                {chats === null ? (
                    [0, 1, 2, 3].map((i) => <Skeleton key={i} height={56} />)
                ) : chats.length === 0 ? (
                    <EmptyState compact icon={<ChatBubble sx={{ fontSize: 32 }} />} title={t('readonlyChats.empty')} />
                ) : (
                    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: radius.card, overflow: 'hidden' }}>
                        {chats.map((c) => (
                            <Box
                                key={c.id}
                                component="a"
                                href={paths.chat ? paths.chat(c.id) : undefined}
                                onClick={(e) => { e.preventDefault(); open(c); }}
                                sx={{ display: 'flex', alignItems: 'center', gap: 1.5, px: 2, py: 1.5, textDecoration: 'none', color: 'inherit', borderTop: 1, borderColor: 'divider', '&:first-of-type': { borderTop: 0 }, '&:hover': { bgcolor: 'action.hover' }, '& .row-del': { opacity: 0 }, '&:hover .row-del': { opacity: 1 } }}
                            >
                                <ChatBubble sx={{ fontSize: 18, color: 'text.secondary', flexShrink: 0 }} />
                                <Box sx={{ flex: 1, minWidth: 0 }}>
                                    <Typography noWrap sx={{ color: 'text.primary', fontWeight: 500, fontSize: fontSizes.base }}>
                                        {c.title || t('readonlyChats.untitled')}
                                    </Typography>
                                    <Typography variant="caption" color="text.secondary">
                                        {t('readonlyChats.updated', { time: timeAgo(c.updated_at, i18n.language) })}
                                        {c.status === 'working' ? ` · ${t('readonlyChats.working')}` : c.status === 'failed' ? ` · ${t('readonlyChats.failed')}` : ''}
                                    </Typography>
                                </Box>
                                <IconButton
                                    className="row-del"
                                    size="small"
                                    aria-label={t('readonlyChats.delete')}
                                    onClick={(e) => { e.preventDefault(); e.stopPropagation(); setDeleting(c); }}
                                    sx={{ color: 'text.secondary' }}
                                >
                                    <Delete fontSize="small" />
                                </IconButton>
                            </Box>
                        ))}
                    </Box>
                )}
            </Box>
            <ConfirmDialog
                open={!!deleting}
                title={t('readonlyChats.deleteTitle')}
                message={t('readonlyChats.deleteMessage', { title: deleting?.title || t('readonlyChats.untitled') })}
                confirmLabel={t('readonlyChats.delete')}
                danger
                onClose={() => setDeleting(null)}
                onConfirm={() => void remove()}
            />
        </Box>
    );
}
