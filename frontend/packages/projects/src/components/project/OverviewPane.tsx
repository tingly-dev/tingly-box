import { useEffect, useRef, useState } from 'react';
import { Box, Button, Chip, IconButton, LinearProgress, Tab, Tabs, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { CopyIconButton } from '@tingly/ui';
import { api, apiFetch, unwrap, type LibraryFile, type Thread } from '../../client';
import { Delete, Description, Download, UploadFile } from '../icons';
import { useNotify } from '../../hooks/useNotify';
import { formatBytes, timeAgo } from '../../utils/format';
import { STATUS_ORDER, ThreadBadges, ThreadSubtitle, needsUser } from './threadStatus';
import { StatusGroups } from '../workspace/StatusGroups';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';

const mono = { fontFamily: fontMono } as const;

function ThreadGroups({ threads, selectedId, onOpen }: { threads: Thread[]; selectedId?: string; onOpen: (id: string) => void }) {
    const { t, i18n } = useTranslation('projects');
    const groups = STATUS_ORDER.map((status) => ({
        key: status, label: t(`overview.group.${status}`), collapsed: status === 'resolved',
    }));
    const rows = threads.map((th) => ({
        id: th.id, group: th.status, title: th.title, selected: th.id === selectedId,
        subtitle: <ThreadSubtitle thread={th} />, badges: <ThreadBadges thread={th} />, meta: timeAgo(th.updated_at, i18n.language),
    }));
    return <StatusGroups groups={groups} rows={rows} onOpen={onOpen} empty={t('overview.noThreads')} />;
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
                <Box key={f.path} sx={{ display: 'flex', alignItems: 'center', gap: 1, py: 0.75, px: 1, borderRadius: radius.card, '&:hover': { bgcolor: 'action.hover' }, '& .act': { opacity: 0 }, '&:hover .act': { opacity: 1 } }}>
                    <Description sx={{ fontSize: 18, color: 'text.secondary' }} />
                    <Box sx={{ flex: 1, minWidth: 0 }}>
                        <Typography noWrap sx={{ fontSize: fontSizes.base, color: 'text.primary', ...mono }}>{f.path}</Typography>
                        <Typography variant="caption" color="text.secondary">{formatBytes(f.size)} · {timeAgo(f.updated_at, i18n.language)}</Typography>
                    </Box>
                    <Chip size="small" variant="outlined" label={t(`library.origin.${f.origin}`)} sx={{ height: 20, fontSize: fontSizes.xs }} />
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

export function OverviewPane({ projectId, threads, onOpen, libraryKey, selectedId, initialTab = 'threads' }: { projectId: string; threads: Thread[]; onOpen: (id: string) => void; libraryKey: number; selectedId?: string; initialTab?: 'threads' | 'library' }) {
    const { t } = useTranslation('projects');
    const [tab, setTab] = useState<'threads' | 'library'>(initialTab);
    const waiting = threads.filter((th) => needsUser(th.status)).length;
    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}>
            <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ px: 1, minHeight: 44, borderBottom: 1, borderColor: 'divider', '& .MuiTab-root': { minHeight: 44 } }}>
                <Tab value="threads" label={waiting > 0 ? t('overview.threadsWaiting', { count: waiting }) : t('overview.threads')} />
                <Tab value="library" label={t('overview.library')} />
            </Tabs>
            <Box sx={{ flex: 1, overflowY: 'auto' }}>
                {tab === 'threads' ? <ThreadGroups threads={threads} selectedId={selectedId} onOpen={onOpen} /> : <Library projectId={projectId} refreshKey={libraryKey} />}
            </Box>
        </Box>
    );
}
