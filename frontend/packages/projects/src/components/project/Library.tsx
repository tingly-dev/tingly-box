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
import { LibraryPreview, previewKind, type PreviewFile } from './LibraryPreview';

const mono = { fontFamily: fontMono } as const;

type Node = LibraryEntry & { kind: 'dir' | 'file' };

/** An upload is stored as uploads/<12 hex id>-<name>; it is saved under the name it was given. */
export const downloadName = (path: string): string => {
    const base = path.split('/').pop() || path;
    return /^uploads\/[0-9a-f]{12}-[^/]+$/.test(path) ? base.slice(13) : base;
};

const toPreview = (n: Node): PreviewFile => ({ path: n.path, name: n.name, size: n.size });

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
 * contents are fetched when it is opened; files can be previewed (images, Markdown and text), downloaded or deleted, and a folder downloaded as a zip.
 */
export function Library({ projectId, refreshKey }: { projectId: string; refreshKey: number }) {
    const { t, i18n } = useTranslation('projects');
    const notify = useNotify();
    const input = useRef<HTMLInputElement>(null);
    const [dir, setDir] = useState('');
    const [uploading, setUploading] = useState(false);
    const [tick, setTick] = useState(0);
    const [expanded, setExpanded] = useExpanded(projectId);
    const [preview, setPreview] = useState<PreviewFile | null>(null);
    // The images of the previewed file's folder, in the order the tree lists them.
    const [images, setImages] = useState<PreviewFile[]>([]);
    const listed = useRef(new Map<string, Node[]>());

    const load = useCallback(async (path: string): Promise<Node[]> => {
        const res = await apiFetch(`/api/v1/projects/${projectId}/library/tree?path=${encodeURIComponent(path)}`);
        if (!res.ok) throw new Error(((await res.json().catch(() => ({}))) as { error?: string }).error ?? res.statusText);
        const tree = (await res.json()) as LibraryTree;
        if (path === '') setDir(tree.dir);
        listed.current.set(path, tree.entries as Node[]);
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
    // authentication applies to downloads too, and a refusal (a folder over
    // the zip limits, say) is shown as the server's own message.
    const save = async (url: string, filename: string) => {
        try {
            const res = await apiFetch(url);
            if (!res.ok) throw new Error(((await res.json().catch(() => ({}))) as { error?: string }).error ?? res.statusText);
            const blobUrl = URL.createObjectURL(await res.blob());
            const a = document.createElement('a');
            a.href = blobUrl;
            a.download = filename;
            a.click();
            setTimeout(() => URL.revokeObjectURL(blobUrl), 0);
        } catch (e) {
            notify((e as Error).message, 'error');
        }
    };
    const download = (path: string) => save(`/api/v1/projects/${projectId}/library/file?path=${encodeURIComponent(path)}`, downloadName(path));
    const downloadZip = (n: Node) => save(`/api/v1/projects/${projectId}/library/archive?path=${encodeURIComponent(n.path)}`, `${n.name}.zip`);

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
                onActivate={(n) => {
                    const kind = previewKind(n.name);
                    if (!kind) return;
                    const folder = n.path.includes('/') ? n.path.slice(0, n.path.lastIndexOf('/')) : '';
                    setImages(kind === 'image' ? (listed.current.get(folder) ?? []).filter((e) => e.kind === 'file' && previewKind(e.name) === 'image').map(toPreview) : []);
                    setPreview(toPreview(n));
                }}
                empty={<Typography variant="body2" color="text.secondary" sx={{ py: 2 }}>{t('library.empty')}</Typography>}
                renderMeta={(n) => n.kind === 'file' ? `${formatBytes(n.size)} · ${timeAgo(n.updated_at, i18n.language, true)}` : null}
                renderActions={(n, { tabIndex }) => n.kind === 'dir' ? (
                    <Tooltip title={t('library.downloadZip')}>
                        <IconButton size="small" tabIndex={tabIndex} aria-label={t('library.downloadZip')} onClick={() => void downloadZip(n)}><Download fontSize="small" /></IconButton>
                    </Tooltip>
                ) : (
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
            <LibraryPreview projectId={projectId} file={preview} siblings={images} onNavigate={setPreview} onClose={() => setPreview(null)} onDownload={(p) => void download(p)} />
        </Box>
    );
}
