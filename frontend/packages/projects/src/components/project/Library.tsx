import { useCallback, useMemo, useRef, useState } from 'react';
import { Box, Button, IconButton, InputAdornment, LinearProgress, MenuItem, Select, TextField, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { CopyIconButton } from '@tingly/ui';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { api, apiFetch, unwrap, type LibraryEntry, type LibraryTree } from '../../client';
import { Delete, Download, Search, UploadFile } from '../icons';
import { useNotify } from '../../hooks/useNotify';
import { readLocal, writeLocal } from '../../utils/storage';
import { formatBytes, timeAgo } from '../../utils/format';
import { precheckUploads, uploadLimitLabel } from '../../utils/uploadLimit';
import { LazyTree } from '../common/LazyTree';
import { LibraryPreview, previewKind, saveFile, type PreviewFile } from './LibraryPreview';
import { displayName, LIBRARY_SORTS, matchesQuery, sortLibraryEntries, type LibrarySort } from './libraryEntries';

const mono = { fontFamily: fontMono } as const;

type Node = LibraryEntry & { kind: 'dir' | 'file' };

const toPreview = (n: Node): PreviewFile => ({ path: n.path, name: n.name, size: n.size });

/** A browser remembers the last order picked; time, newest first, is the default. */
const readSort = (): LibrarySort => {
    const saved = readLocal('library.sort');
    return LIBRARY_SORTS.includes(saved as LibrarySort) ? (saved as LibrarySort) : 'time-desc';
};

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
    const [sort, setSort] = useState<LibrarySort>(readSort);
    const [query, setQuery] = useState('');
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
        // The API returns stored names; an upload's stored id is smoothed out
        // for display here in the browser.
        const entries = (tree.entries as Node[]).map((n) => n.kind === 'file' ? { ...n, name: displayName(n.path) } : n);
        listed.current.set(path, entries);
        return entries;
    }, [projectId]);

    // Ordering is a browser preference and happens over what is loaded, so
    // changing it neither reloads nor re-requests anything.
    const sortItems = useCallback((items: Node[]) => sortLibraryEntries(items, sort), [sort]);
    const filter = useMemo(() => {
        const q = query.trim();
        return q ? (n: Node) => matchesQuery(n.name, q) : undefined;
    }, [query]);

    const changeSort = (next: LibrarySort) => {
        writeLocal('library.sort', next);
        setSort(next);
    };

    const upload = async (list: FileList) => {
        // Refuse an oversized file before its bytes go up, naming it and the
        // limit, rather than after the server answers 413.
        const { accepted, rejected } = precheckUploads(Array.from(list));
        if (rejected.length) {
            notify(rejected.map((r) => t('library.uploadTooLarge', {
                name: r.file.name || 'file',
                size: formatBytes(r.size),
                limit: uploadLimitLabel(),
                over: formatBytes(r.over),
            })).join('\n'), 'error');
        }
        const form = new FormData();
        accepted.forEach((f) => form.append('files', f));
        if (accepted.length === 0) {
            if (input.current) input.current.value = '';
            return;
        }
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

    // Through the shared save (apiFetch rather than a plain link) so the host's
    // authentication applies to downloads too, and a refusal (a folder over
    // the zip limits, say) is shown as the server's own message.
    const save = (url: string, filename: string) => saveFile(url, filename, (m) => notify(m, 'error'));
    // A row already shows the readable name; path-only callers derive it.
    const download = (path: string, name?: string) => save(`/api/v1/projects/${projectId}/library/file?path=${encodeURIComponent(path)}`, name ?? displayName(path));
    const downloadZip = (n: Node) => save(`/api/v1/projects/${projectId}/library/archive?path=${encodeURIComponent(n.path)}`, `${n.name}.zip`);

    return (
        <Box sx={{ p: 1.5 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                <Typography variant="caption" color="text.secondary" sx={{ flex: 1 }}>{t('library.intro')}</Typography>
            </Box>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                <TextField
                    size="small"
                    variant="standard"
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    placeholder={t('library.search')}
                    slotProps={{
                        htmlInput: { 'aria-label': t('library.search') },
                        input: { startAdornment: <InputAdornment position="start"><Search fontSize="small" /></InputAdornment> },
                    }}
                    sx={{ flex: 1, minWidth: 0 }}
                />
                <Select
                    size="small"
                    variant="standard"
                    value={sort}
                    onChange={(e) => changeSort(e.target.value as LibrarySort)}
                    inputProps={{ 'aria-label': t('library.sort') }}
                    sx={{ fontSize: fontSizes.sm }}
                >
                    <MenuItem value="time-desc">{t('library.sortTimeDesc')}</MenuItem>
                    <MenuItem value="time-asc">{t('library.sortTimeAsc')}</MenuItem>
                    <MenuItem value="name-asc">{t('library.sortNameAsc')}</MenuItem>
                    <MenuItem value="name-desc">{t('library.sortNameDesc')}</MenuItem>
                </Select>
                <Tooltip title={t('library.addHint', { limit: uploadLimitLabel() })}>
                    <Button size="small" startIcon={<UploadFile />} onClick={() => input.current?.click()}>{t('library.add')}</Button>
                </Tooltip>
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
            <input ref={input} type="file" multiple hidden data-testid="library-file-input" onChange={(e) => e.target.files && void upload(e.target.files)} />
            <LazyTree<Node>
                ariaLabel={t('library.tree')}
                load={load}
                expanded={expanded}
                onExpandedChange={setExpanded}
                refreshKey={refreshKey + tick}
                sortItems={sortItems}
                filter={filter}
                onActivate={(n) => {
                    const kind = previewKind(n.name);
                    if (!kind) return;
                    const folder = n.path.includes('/') ? n.path.slice(0, n.path.lastIndexOf('/')) : '';
                    setImages(kind === 'image' ? (listed.current.get(folder) ?? []).filter((e) => e.kind === 'file' && previewKind(e.name) === 'image').map(toPreview) : []);
                    setPreview(toPreview(n));
                }}
                empty={<Typography variant="body2" color="text.secondary" sx={{ py: 2 }}>{t('library.empty')}</Typography>}
                noResults={<Typography variant="body2" color="text.secondary" sx={{ py: 2 }}>{t('library.noResults')}</Typography>}
                renderMeta={(n) => {
                    // Scratch (a thread's TMPDIR under .tmp/) is labelled, but only down
                    // to the thread folder: everything below it is scratch anyway, and
                    // repeating the label on every level is noise.
                    const depth = n.path.split('/').length;
                    const scratch = n.origin === 'scratch' && depth <= 2 ? t('library.scratch') : null;
                    if (n.kind === 'file') {
                        const meta = `${formatBytes(n.size)} · ${timeAgo(n.updated_at, i18n.language, true)}`;
                        return scratch ? `${scratch} · ${meta}` : meta;
                    }
                    return scratch;
                }}
                renderActions={(n, { tabIndex }) => n.kind === 'dir' ? (
                    <Tooltip title={t('library.downloadZip')}>
                        <IconButton size="small" tabIndex={tabIndex} aria-label={t('library.downloadZip')} onClick={() => void downloadZip(n)}><Download fontSize="small" /></IconButton>
                    </Tooltip>
                ) : (
                    <>
                        <Tooltip title={t('library.download')}>
                            <IconButton size="small" tabIndex={tabIndex} aria-label={t('library.download')} onClick={() => void download(n.path, n.name)}><Download fontSize="small" /></IconButton>
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
