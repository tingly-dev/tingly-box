import { useEffect, useRef, useState } from 'react';
import { Box, Button, Chip, Collapse, IconButton, LinearProgress, Tab, Tabs, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { CopyIconButton } from '@tingly/ui';
import { api, apiFetch, unwrap, type LibraryFile, type Thread } from '../../client';
import { Delete, Description, Download, ExpandMore, UploadFile } from '../icons';
import { useNotify } from '../../hooks/useNotify';
import { formatBytes, timeAgo } from '../../utils/format';
import { STATUS_ORDER, StatusHeading, ThreadSubtitle } from './threadStatus';

const mono = { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' } as const;

function ThreadRow({ thread, onOpen }: { thread: Thread; onOpen: (id: string) => void }) {
    const { i18n } = useTranslation('projects');
    return (
        <Box
            role="button"
            tabIndex={0}
            onClick={() => onOpen(thread.id)}
            onKeyDown={(e) => e.key === 'Enter' && onOpen(thread.id)}
            sx={{ px: 1.5, py: 1, borderRadius: 2, '&:hover': { bgcolor: 'action.hover' }, outline: 'none' }}
        >
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Typography noWrap sx={{ flex: 1, minWidth: 0, fontSize: 14, fontWeight: 500, color: 'text.primary' }}>{thread.title}</Typography>
                <Typography variant="caption" color="text.secondary" sx={{ flexShrink: 0 }}>{timeAgo(thread.updated_at, i18n.language)}</Typography>
            </Box>
            <Typography variant="caption" color="text.secondary" noWrap component="div"><ThreadSubtitle thread={thread} /></Typography>
        </Box>
    );
}

function ThreadGroups({ threads, onOpen }: { threads: Thread[]; onOpen: (id: string) => void }) {
    const { t } = useTranslation('projects');
    const [showResolved, setShowResolved] = useState(false);
    if (threads.length === 0) {
        return <Typography variant="body2" color="text.secondary" sx={{ p: 2 }}>{t('overview.noThreads')}</Typography>;
    }
    return (
        <Box sx={{ py: 1 }}>
            {STATUS_ORDER.map((status) => {
                const list = threads.filter((th) => th.status === status);
                if (list.length === 0) return null;
                const collapsible = status === 'resolved';
                return (
                    <Box key={status} sx={{ mb: 1.5 }}>
                        <Box
                            role={collapsible ? 'button' : undefined}
                            onClick={collapsible ? () => setShowResolved((v) => !v) : undefined}
                            sx={{ display: 'flex', alignItems: 'center', gap: 1, px: 1.5, py: 0.5 }}
                        >
                            <StatusHeading status={status} count={list.length} />
                            {collapsible && <ExpandMore sx={{ fontSize: 16, color: 'text.secondary', transform: showResolved ? 'rotate(180deg)' : 'none' }} />}
                        </Box>
                        <Collapse in={!collapsible || showResolved}>
                            {list.map((th) => <ThreadRow key={th.id} thread={th} onOpen={onOpen} />)}
                        </Collapse>
                    </Box>
                );
            })}
        </Box>
    );
}

function Library({ projectId, refreshKey }: { projectId: string; refreshKey: number }) {
    const { t, i18n } = useTranslation('projects');
    const notify = useNotify();
    const input = useRef<HTMLInputElement>(null);
    const [files, setFiles] = useState<LibraryFile[] | null>(null);
    const [dir, setDir] = useState('');
    const [uploading, setUploading] = useState(false);
    const [tick, setTick] = useState(0);

    useEffect(() => {
        unwrap(api.GET('/api/v1/projects/{id}/library', { params: { path: { id: projectId } } }))
            .then((r) => {
                setFiles(r.files);
                setDir(r.dir);
            })
            .catch((e: Error) => notify(e.message, 'error'));
    }, [projectId, refreshKey, tick, notify]);

    const upload = async (list: FileList) => {
        const form = new FormData();
        Array.from(list).forEach((f) => form.append('files', f));
        setUploading(true);
        try {
            const res = await apiFetch(`/api/v1/projects/${projectId}/library`, { method: 'POST', body: form });
            if (!res.ok) throw new Error(((await res.json()) as { error?: string }).error ?? res.statusText);
            setTick((x) => x + 1);
        } catch (e) {
            notify((e as Error).message, 'error');
        } finally {
            setUploading(false);
            if (input.current) input.current.value = '';
        }
    };

    // Through apiFetch rather than a plain link, so the host's
    // authentication applies to downloads too.
    const download = async (path: string) => {
        try {
            const res = await apiFetch(`/api/v1/projects/${projectId}/library/file?path=${encodeURIComponent(path)}`);
            if (!res.ok) throw new Error(((await res.json().catch(() => ({}))) as { error?: string }).error ?? res.statusText);
            const url = URL.createObjectURL(await res.blob());
            const a = document.createElement('a');
            a.href = url;
            a.download = path.split('/').pop() || path;
            a.click();
            setTimeout(() => URL.revokeObjectURL(url), 0);
        } catch (e) {
            notify((e as Error).message, 'error');
        }
    };

    return (
        <Box sx={{ p: 1.5 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                <Typography variant="caption" color="text.secondary" sx={{ flex: 1 }}>{t('library.intro')}</Typography>
                <Button size="small" startIcon={<UploadFile />} onClick={() => input.current?.click()}>{t('library.add')}</Button>
            </Box>
            {dir && (
                // One line, clipped at the start so the project's own folder stays in view.
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 1 }}>
                    <Tooltip title={dir}>
                        <Typography variant="caption" noWrap sx={{ ...mono, flex: 1, minWidth: 0, color: 'text.disabled', direction: 'rtl', textAlign: 'left' }}>
                            <bdi>{dir}</bdi>
                        </Typography>
                    </Tooltip>
                    <CopyIconButton value={dir} label={t('library.copyPath')} copiedLabel={t('common.copied')} iconSize={14} />
                </Box>
            )}
            {uploading && <LinearProgress sx={{ mb: 1 }} />}
            <input ref={input} type="file" multiple hidden onChange={(e) => e.target.files && void upload(e.target.files)} />
            {files?.length === 0 && <Typography variant="body2" color="text.secondary" sx={{ py: 2 }}>{t('library.empty')}</Typography>}
            {files?.map((f) => (
                <Box key={f.path} sx={{ display: 'flex', alignItems: 'center', gap: 1, py: 0.75, px: 1, borderRadius: 1.5, '&:hover': { bgcolor: 'action.hover' }, '& .act': { opacity: 0 }, '&:hover .act': { opacity: 1 } }}>
                    <Description sx={{ fontSize: 18, color: 'text.secondary' }} />
                    <Box sx={{ flex: 1, minWidth: 0 }}>
                        <Typography noWrap sx={{ fontSize: 13.5, color: 'text.primary', ...mono }}>{f.path}</Typography>
                        <Typography variant="caption" color="text.secondary">{formatBytes(f.size)} · {timeAgo(f.updated_at, i18n.language)}</Typography>
                    </Box>
                    <Chip size="small" variant="outlined" label={t(`library.origin.${f.origin}`)} sx={{ height: 20, fontSize: 11 }} />
                    <Tooltip title={t('library.download')}>
                        <IconButton className="act" size="small" onClick={() => void download(f.path)}><Download fontSize="small" /></IconButton>
                    </Tooltip>
                    <Tooltip title={t('common.delete')}>
                        <IconButton className="act" size="small" onClick={() => {
                            unwrap(api.DELETE('/api/v1/projects/{id}/library/file', { params: { path: { id: projectId }, query: { path: f.path } } }))
                                .then(() => setTick((x) => x + 1))
                                .catch((e: Error) => notify(e.message, 'error'));
                        }}><Delete fontSize="small" /></IconButton>
                    </Tooltip>
                </Box>
            ))}
        </Box>
    );
}

export function OverviewPane({ projectId, threads, onOpen, libraryKey }: { projectId: string; threads: Thread[]; onOpen: (id: string) => void; libraryKey: number }) {
    const { t } = useTranslation('projects');
    const [tab, setTab] = useState<'threads' | 'library'>('threads');
    const waiting = threads.filter((th) => th.status === 'waiting').length;
    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}>
            <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ px: 1, minHeight: 44, borderBottom: 1, borderColor: 'divider', '& .MuiTab-root': { minHeight: 44 } }}>
                <Tab value="threads" label={waiting > 0 ? t('overview.threadsWaiting', { count: waiting }) : t('overview.threads')} />
                <Tab value="library" label={t('overview.library')} />
            </Tabs>
            <Box sx={{ flex: 1, overflowY: 'auto' }}>
                {tab === 'threads' ? <ThreadGroups threads={threads} onOpen={onOpen} /> : <Library projectId={projectId} refreshKey={libraryKey} />}
            </Box>
        </Box>
    );
}
