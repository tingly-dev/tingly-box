import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Alert, Box, Button, Checkbox, Chip, FormControlLabel, IconButton, InputAdornment, Skeleton, TextField, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import { api, unwrap, type Diff, type DiffFile, type FileDiff } from '../../../client';
import { ArrowForward, KeyboardArrowDown, KeyboardArrowUp, Search } from '../../icons';
import { DiffFileList, splitPath } from './DiffFileList';
import { DiffPatchView } from './DiffPatchView';
import { filterFiles, firstReviewable, LARGE_FILE_LINES, loadViewed, parsePatch, saveViewed, viewedSignature, type ParsedPatch, type ViewedMap } from './diffModel';

type Loaded =
    | { state: 'loading' }
    | { state: 'error'; message: string }
    | { state: 'ok'; file: FileDiff; parsed: ParsedPatch };

const changedLines = (f: DiffFile) => f.additions + f.deletions;
const isLarge = (f: DiffFile) => changedLines(f) > LARGE_FILE_LINES;
const fmt = (n: number) => n.toLocaleString();
// Parsed patches kept in memory; a forced one can be megabytes.
const KEEP_PATCHES = 6;

/** A thread's changes: the file list on one side, one file's patch on the
 * other. Nothing is rendered or fetched for a file until it is opened, so a
 * change of any size opens at once; lock files, binaries and huge diffs wait
 * behind a button. */
export function DiffView({ threadId, diff }: { threadId: string; diff: Diff }) {
    const { t } = useTranslation('projects');
    const files = diff.files;
    const [query, setQuery] = useState('');
    const [hideGenerated, setHideGenerated] = useState(false);
    const [unviewedOnly, setUnviewedOnly] = useState(false);
    const [viewed, setViewed] = useState<ViewedMap>(() => loadViewed(threadId));
    const [opened, setOpened] = useState<Set<string>>(() => new Set());
    const [loaded, setLoaded] = useState<Record<string, Loaded>>({});
    const requested = useRef(new Set<string>());
    const loadOrder = useRef<string[]>([]);

    const isViewed = useCallback((f: DiffFile) => viewed[f.path] === viewedSignature(f), [viewed]);
    const viewedSet = useMemo(() => new Set(files.filter(isViewed).map((f) => f.path)), [files, isViewed]);
    const indexes = useMemo(
        () => filterFiles(files, { query, hideGenerated, unviewedOnly }, isViewed),
        [files, query, hideGenerated, unviewedOnly, isViewed],
    );
    const [selectedPath, setSelectedPath] = useState<string | null>(() => {
        const i = firstReviewable(files, files.map((_, k) => k));
        return i >= 0 ? files[i].path : null;
    });
    const selected = useMemo(() => files.find((f) => f.path === selectedPath) ?? null, [files, selectedPath]);

    const load = useCallback((file: DiffFile, force: boolean) => {
        const key = `${file.path}${force ? '#force' : ''}`;
        if (requested.current.has(key)) return;
        requested.current.add(key);
        setLoaded((m) => ({ ...m, [file.path]: { state: 'loading' } }));
        unwrap(api.GET('/api/v1/threads/{id}/diff/file', { params: { path: { id: threadId }, query: { path: file.path, old_path: file.old_path || undefined, force: force || undefined } } }))
            .then((data) => {
                const parsed = parsePatch(data.patch);
                loadOrder.current = [...loadOrder.current.filter((p) => p !== file.path), file.path];
                setLoaded((m) => {
                    const next = { ...m, [file.path]: { state: 'ok', file: data, parsed } satisfies Loaded };
                    // Patches pile up as files are paged through; keep the recent ones.
                    while (loadOrder.current.length > KEEP_PATCHES) {
                        const old = loadOrder.current.shift() as string;
                        delete next[old];
                        requested.current.delete(old);
                        requested.current.delete(`${old}#force`);
                    }
                    return next;
                });
            })
            .catch((e: Error) => {
                requested.current.delete(key);
                setLoaded((m) => ({ ...m, [file.path]: { state: 'error', message: e.message } }));
            });
    }, [threadId]);

    // Whether a file's patch is fetched without being asked for.
    const autoLoad = useCallback((f: DiffFile) => !f.binary && !(f.generated && !opened.has(f.path)) && !(isLarge(f) && !opened.has(f.path)), [opened]);

    useEffect(() => {
        if (selected && autoLoad(selected) && !loaded[selected.path]) load(selected, opened.has(selected.path) && isLarge(selected));
    }, [selected, autoLoad, loaded, load, opened]);

    // The next file is fetched while the current one is read.
    useEffect(() => {
        if (selectedPath && loadOrder.current.includes(selectedPath)) loadOrder.current = [...loadOrder.current.filter((p) => p !== selectedPath), selectedPath];
    }, [selectedPath]);

    const nextPath = useMemo(() => {
        const pos = indexes.findIndex((i) => files[i].path === selectedPath);
        return pos >= 0 && pos + 1 < indexes.length ? files[indexes[pos + 1]].path : null;
    }, [indexes, files, selectedPath]);
    useEffect(() => {
        const next = files.find((f) => f.path === nextPath);
        if (next && autoLoad(next) && !loaded[next.path]) load(next, false);
    }, [nextPath, files, autoLoad, loaded, load]);

    const toggleViewed = useCallback((f: DiffFile) => {
        setViewed((m) => {
            const next = { ...m };
            if (next[f.path] === viewedSignature(f)) delete next[f.path];
            else next[f.path] = viewedSignature(f);
            saveViewed(threadId, next);
            return next;
        });
    }, [threadId]);

    const step = useCallback((delta: 1 | -1, unviewed = false) => {
        const pos = indexes.findIndex((i) => files[i].path === selectedPath);
        for (let p = pos + delta; p >= 0 && p < indexes.length; p += delta) {
            const f = files[indexes[p]];
            if (!unviewed || !isViewed(f)) {
                setSelectedPath(f.path);
                return;
            }
        }
    }, [indexes, files, selectedPath, isViewed]);

    useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.metaKey || e.ctrlKey || e.altKey) return;
            const target = e.target as HTMLElement | null;
            if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) return;
            if (e.key === 'j') step(1);
            else if (e.key === 'k') step(-1);
            else if (e.key === 'v' && selected) {
                const wasViewed = isViewed(selected);
                toggleViewed(selected);
                if (!wasViewed) step(1, true);
            } else return;
            e.preventDefault();
        };
        window.addEventListener('keydown', onKey);
        return () => window.removeEventListener('keydown', onKey);
    }, [step, selected, isViewed, toggleViewed]);

    const open = (f: DiffFile) => {
        setOpened((s) => new Set(s).add(f.path));
        load(f, isLarge(f));
    };

    if (files.length === 0) return <Typography color="text.secondary">{t('thread.noChanges')}</Typography>;

    const selPos = indexes.findIndex((i) => files[i].path === selectedPath);
    const entry = selected ? loaded[selected.path] : undefined;

    return (
        <Box sx={{ display: 'flex', flexDirection: { xs: 'column', md: 'row' }, height: { xs: '68vh', md: '64vh' }, minHeight: 320, border: 1, borderColor: 'divider', borderRadius: radius.row, overflow: 'hidden' }}>
            <Box sx={{ display: 'flex', flexDirection: 'column', width: { md: 340 }, flexShrink: 0, height: { xs: '36%', md: 'auto' }, borderRight: { md: 1 }, borderBottom: { xs: 1, md: 0 }, borderColor: 'divider' }}>
                <Box sx={{ p: 1, display: 'flex', flexDirection: 'column', gap: 0.75, borderBottom: 1, borderColor: 'divider' }}>
                    <Typography sx={{ fontSize: fontSizes.sm, color: 'text.secondary' }}>
                        {t('thread.files', { count: files.length })} · <Box component="span" sx={{ color: 'success.main' }}>+{fmt(diff.additions)}</Box> <Box component="span" sx={{ color: 'error.main' }}>−{fmt(diff.deletions)}</Box> · {t('thread.diff.progress', { viewed: viewedSet.size, total: files.length })}
                    </Typography>
                    <TextField size="small" value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t('thread.diff.filterPlaceholder')}
                        slotProps={{ htmlInput: { 'aria-label': t('thread.diff.filterPlaceholder') }, input: { startAdornment: <InputAdornment position="start"><Search fontSize="small" /></InputAdornment> } }} />
                    {!!diff.files_omitted && <Typography sx={{ fontSize: fontSizes.sm, color: 'warning.main' }}>{t('thread.diff.omitted', { count: diff.files_omitted })}</Typography>}
                    <Box sx={{ display: 'flex', gap: 0.75, flexWrap: 'wrap' }}>
                        <Chip size="small" clickable variant={hideGenerated ? 'filled' : 'outlined'} color={hideGenerated ? 'primary' : 'default'} label={t('thread.diff.hideGenerated')} onClick={() => setHideGenerated((v) => !v)} />
                        <Chip size="small" clickable variant={unviewedOnly ? 'filled' : 'outlined'} color={unviewedOnly ? 'primary' : 'default'} label={t('thread.diff.unviewedOnly')} onClick={() => setUnviewedOnly((v) => !v)} />
                    </Box>
                </Box>
                {indexes.length === 0
                    ? <Typography sx={{ p: 2, fontSize: fontSizes.md, color: 'text.secondary' }}>{t('thread.diff.noMatch')}</Typography>
                    : <DiffFileList files={files} indexes={indexes} selectedPath={selectedPath} viewedSet={viewedSet} onSelect={setSelectedPath} onToggleViewed={toggleViewed} />}
                <Typography sx={{ display: { xs: 'none', md: 'block' }, px: 1, py: 0.5, fontSize: fontSizes.micro, color: 'text.secondary', borderTop: 1, borderColor: 'divider' }}>{t('thread.diff.keys')}</Typography>
            </Box>

            <Box sx={{ flex: 1, minWidth: 0, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
                {!selected ? (
                    <Typography sx={{ p: 2, color: 'text.secondary' }}>{t('thread.diff.selectFile')}</Typography>
                ) : (
                    <>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, px: 1.5, py: 0.5, minHeight: 40, borderBottom: 1, borderColor: 'divider' }}>
                            <Box sx={{ flex: 1, minWidth: 0 }}>
                                <Typography noWrap title={selected.path} sx={{ fontFamily: fontMono, fontSize: fontSizes.md, fontWeight: 600 }}>
                                    {selected.old_path && <><Box component="span" sx={{ color: 'text.secondary', fontWeight: 400 }}>{splitPath(selected.old_path).name}</Box> <ArrowForward sx={{ fontSize: 12, verticalAlign: 'middle' }} /> </>}
                                    {selected.path}
                                </Typography>
                                {selected.old_path && <Typography noWrap sx={{ fontSize: fontSizes.micro, color: 'text.secondary' }}>{t('thread.diff.renamedFrom', { path: selected.old_path })}</Typography>}
                            </Box>
                            <Typography component="span" sx={{ fontFamily: fontMono, fontSize: fontSizes.sm, whiteSpace: 'nowrap' }}>
                                <Box component="span" sx={{ color: 'success.main' }}>+{fmt(selected.additions)}</Box> <Box component="span" sx={{ color: 'error.main' }}>−{fmt(selected.deletions)}</Box>
                            </Typography>
                            <FormControlLabel sx={{ m: 0, '& .MuiFormControlLabel-label': { fontSize: fontSizes.sm } }} label={t('thread.diff.viewed')}
                                control={<Checkbox size="small" checked={isViewed(selected)} onChange={() => toggleViewed(selected)} />} />
                            <Tooltip title={t('thread.diff.prevFile')}><span><IconButton size="small" aria-label={t('thread.diff.prevFile')} disabled={selPos <= 0} onClick={() => step(-1)}><KeyboardArrowUp fontSize="small" /></IconButton></span></Tooltip>
                            <Tooltip title={t('thread.diff.nextFile')}><span><IconButton size="small" aria-label={t('thread.diff.nextFile')} disabled={selPos < 0 || selPos >= indexes.length - 1} onClick={() => step(1)}><KeyboardArrowDown fontSize="small" /></IconButton></span></Tooltip>
                        </Box>
                        <Box key={selected.path} sx={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
                            <FileBody file={selected} entry={entry} autoLoad={autoLoad(selected)} onOpen={() => open(selected)} onForce={() => load(selected, true)} onRetry={() => load(selected, opened.has(selected.path) && isLarge(selected))} />
                        </Box>
                    </>
                )}
            </Box>
        </Box>
    );
}

function Notice({ title, body, action, onAction }: { title: string; body: string; action?: string; onAction?: () => void }) {
    return (
        <Box sx={{ m: 2, p: 2, display: 'flex', alignItems: 'center', gap: 2, bgcolor: 'action.hover', borderRadius: radius.row }}>
            <Box sx={{ flex: 1 }}>
                <Typography sx={{ fontSize: fontSizes.base, fontWeight: 600 }}>{title}</Typography>
                <Typography sx={{ fontSize: fontSizes.md, color: 'text.secondary' }}>{body}</Typography>
            </Box>
            {action && <Button size="small" variant="outlined" onClick={onAction}>{action}</Button>}
        </Box>
    );
}

function FileBody({ file, entry, autoLoad, onOpen, onForce, onRetry }: {
    file: DiffFile; entry: Loaded | undefined; autoLoad: boolean; onOpen: () => void; onForce: () => void; onRetry: () => void;
}) {
    const { t } = useTranslation('projects');
    const lines = fmt(changedLines(file));
    if (file.binary) return <Notice title={t('thread.diff.binary')} body="" />;
    if (!autoLoad && !entry) {
        return file.generated
            ? <Notice title={t('thread.diff.generatedTitle')} body={t('thread.diff.generatedBody', { lines })} action={t('thread.diff.showDiff')} onAction={onOpen} />
            : <Notice title={t('thread.diff.largeTitle')} body={t('thread.diff.largeBody', { lines })} action={t('thread.diff.loadAnyway')} onAction={onOpen} />;
    }
    if (!entry || entry.state === 'loading') return <Box sx={{ p: 2 }}><Skeleton variant="rounded" height={160} /></Box>;
    if (entry.state === 'error') {
        return <Box sx={{ p: 2 }}><Alert severity="error" action={<Button color="inherit" size="small" onClick={onRetry}>{t('thread.diff.retry')}</Button>}>{t('thread.diff.loadFailed')} {entry.message}</Alert></Box>;
    }
    if (entry.file.too_large) return <Notice title={t('thread.diff.largeTitle')} body={t('thread.diff.tooLargeBody')} action={t('thread.diff.loadAnyway')} onAction={onForce} />;
    if (entry.parsed.text.length === 0) return <Notice title={t('thread.diff.noContent')} body="" />;
    return (
        <>
            <DiffPatchView parsed={entry.parsed} />
            {(entry.file.truncated || !!entry.file.lines_cut) && (
                <Typography sx={{ px: 1.5, py: 0.5, fontSize: fontSizes.sm, color: 'text.secondary', borderTop: 1, borderColor: 'divider' }}>
                    {[entry.file.truncated && t('thread.diff.cutOff'), !!entry.file.lines_cut && t('thread.diff.linesCut', { count: entry.file.lines_cut })].filter(Boolean).join(' ')}
                </Typography>
            )}
        </>
    );
}
