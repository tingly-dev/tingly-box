import { useCallback, useRef, useState } from 'react';
import { Box, Button, IconButton, LinearProgress, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { CopyIconButton } from '@tingly/ui';
import { fontMono } from '@tingly/ui/theme/fonts';
import { api, apiFetch, unwrap, type LibraryEntry, type LibraryTree } from '../../client';
import { Delete, Download, UploadFile } from '../icons';
import { useNotify } from '../../hooks/useNotify';
import { readLocal, writeLocal } from '../../utils/storage';
import { formatBytes, timeAgo } from '../../utils/format';
import { LazyTree } from '../common/LazyTree';

const mono = { fontFamily: fontMono } as const;

type Node = LibraryEntry & { kind: 'dir' | 'file' };

/** The folders left open in a project's library, kept in this browser. */
function useExpanded(projectId: string): [string[], (paths: string[]) => void] {
    const key = `library.expanded.${projectId}`;
    const read = () => {
        try {
            const v: unknown = JSON.parse(readLocal(key) ?? '[]');
            return Array.isArray(v) ? v.filter((p): p is string => typeof p === 'string') : [];
        } catch {
            return [];
        }
    };
    const [state, setState] = useState(() => ({ key, paths: read() }));
    // A different project has its own open folders.
    const paths = state.key === key ? state.paths : read();
    return [paths, (next) => {
        writeLocal(key, JSON.stringify(next));
        setState({ key, paths: next });
    }];
}

/**
 * The project library as a tree of its folders and files. A folder's
 * contents are fetched when it is opened; files can be downloaded or deleted.
 */
export function Library({ projectId, refreshKey }: { projectId: string; refreshKey: number }) {
    const { t, i18n } = useTranslation('projects');
    const notify = useNotify();
    const input = useRef<HTMLInputElement>(null);
    const [dir, setDir] = useState('');
    const [uploading, setUploading] = useState(false);
    const [tick, setTick] = useState(0);
    const [expanded, setExpanded] = useExpanded(projectId);

    const load = useCallback(async (path: string): Promise<Node[]> => {
        const res = await apiFetch(`/api/v1/projects/${projectId}/library/tree?path=${encodeURIComponent(path)}`);
        if (!res.ok) throw new Error(((await res.json().catch(() => ({}))) as { error?: string }).error ?? res.statusText);
        const tree = (await res.json()) as LibraryTree;
        if (path === '') setDir(tree.dir);
        return tree.entries as Node[];
    }, [projectId]);

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
            <LazyTree<Node>
                ariaLabel={t('library.tree')}
                load={load}
                expanded={expanded}
                onExpandedChange={setExpanded}
                refreshKey={refreshKey + tick}
                empty={<Typography variant="body2" color="text.secondary" sx={{ py: 2 }}>{t('library.empty')}</Typography>}
                renderMeta={(n) => n.kind === 'file' ? `${formatBytes(n.size)} · ${timeAgo(n.updated_at, i18n.language, true)}` : null}
                renderActions={(n, { tabIndex }) => n.kind === 'file' && (
                    <>
                        <Tooltip title={t('library.download')}>
                            <IconButton size="small" tabIndex={tabIndex} aria-label={t('library.download')} onClick={() => void download(n.path)}><Download fontSize="small" /></IconButton>
                        </Tooltip>
                        <Tooltip title={t('common.delete')}>
                            <IconButton size="small" tabIndex={tabIndex} aria-label={t('common.delete')} onClick={() => {
                                unwrap(api.DELETE('/api/v1/projects/{id}/library/file', { params: { path: { id: projectId }, query: { path: n.path } } }))
                                    .then(() => setTick((x) => x + 1))
                                    .catch((e: Error) => notify(e.message, 'error'));
                            }}><Delete fontSize="small" /></IconButton>
                        </Tooltip>
                    </>
                )}
            />
        </Box>
    );
}
