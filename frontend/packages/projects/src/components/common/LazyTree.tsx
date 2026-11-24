import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from 'react';
import { Alert, Box, Button, CircularProgress, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import { ChevronRight, Description, Folder, FolderOpen } from '../icons';

export interface TreeNode {
    /** Slash-separated, relative to the root; unique in the tree. */
    path: string;
    name: string;
    kind: 'dir' | 'file';
}

interface Dir<N> { state: 'loading' | 'ok' | 'error'; items: N[]; error?: string }

const parentOf = (path: string) => path.slice(0, Math.max(path.lastIndexOf('/'), 0));

/**
 * A directory tree that loads one level at a time, as folders are opened.
 * The expanded folders are the parent's to keep (so they can be remembered);
 * the component follows the WAI-ARIA tree pattern: one tab stop, arrows to
 * move, Right/Left to open and close, Enter or Space to toggle or activate.
 * `load('')` lists the root.
 */
export function LazyTree<N extends TreeNode>({ ariaLabel, load, expanded, onExpandedChange, refreshKey = 0, onActivate, renderMeta, renderActions, empty }: {
    ariaLabel: string;
    load: (dir: string) => Promise<N[]>;
    expanded: string[];
    onExpandedChange: (paths: string[]) => void;
    /** Changing it reloads every folder that was loaded, keeping what is shown until the answer arrives. */
    refreshKey?: number;
    onActivate?: (node: N) => void;
    renderMeta?: (node: N) => ReactNode;
    /** `tabIndex` is 0 only on the focused row, so the actions are not a tab stop per row. */
    renderActions?: (node: N, row: { tabIndex: 0 | -1 }) => ReactNode;
    /** Shown when the root has no entries. */
    empty?: ReactNode;
}) {
    const { t } = useTranslation('projects');
    const [dirs, setDirs] = useState<Record<string, Dir<N>>>({});
    const [focused, setFocused] = useState('');
    const rows = useRef(new Map<string, HTMLElement>());
    // The latest props, for the async completion below.
    const latest = useRef({ load, expanded, onExpandedChange, failed: t('tree.failed') });
    const failed = t('tree.failed');
    useEffect(() => { latest.current = { load, expanded, onExpandedChange, failed }; });
    const requested = useRef(new Set<string>());
    const seq = useRef(new Map<string, number>());
    const alive = useRef(true);
    useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);

    const fetchDir = useCallback((path: string) => {
        const id = (seq.current.get(path) ?? 0) + 1;
        seq.current.set(path, id);
        // A reload keeps the rows it has; only a first load shows a spinner.
        setDirs((d) => ({ ...d, [path]: d[path]?.state === 'ok' ? d[path] : { state: 'loading', items: [] } }));
        latest.current.load(path).then((items) => {
            if (!alive.current || seq.current.get(path) !== id) return;
            setDirs((d) => ({ ...d, [path]: { state: 'ok', items } }));
            // A folder remembered as open that is no longer there is forgotten.
            const known = new Set(items.filter((n) => n.kind === 'dir').map((n) => n.path));
            const open = latest.current.expanded;
            const kept = open.filter((p) => parentOf(p) !== path || known.has(p));
            if (kept.length !== open.length) latest.current.onExpandedChange(kept);
        }, (e: unknown) => {
            if (!alive.current || seq.current.get(path) !== id) return;
            setDirs((d) => ({ ...d, [path]: { state: 'error', items: [], error: (e as Error).message || latest.current.failed } }));
        });
    }, []);

    // Load the root and every open folder that has a parent listing it.
    useEffect(() => {
        const want = ['', ...expanded.filter((p) => dirs[parentOf(p)]?.state === 'ok' && dirs[parentOf(p)].items.some((n) => n.path === p))];
        for (const p of want) {
            if (requested.current.has(p)) continue;
            requested.current.add(p);
            fetchDir(p);
        }
    }, [expanded, dirs, fetchDir]);

    const shown = useRef(refreshKey);
    useEffect(() => {
        if (shown.current === refreshKey) return;
        shown.current = refreshKey;
        for (const p of requested.current) fetchDir(p);
    }, [refreshKey, fetchDir]);

    const retry = (path: string) => fetchDir(path);
    const toggle = (path: string) => onExpandedChange(expanded.includes(path) ? expanded.filter((p) => p !== path) : [...expanded, path]);

    interface Row { node: N; depth: number; set: number; pos: number }
    type Status = { path: string; depth: number; kind: 'loading' | 'error' | 'empty'; error?: string };
    const flat = useMemo(() => {
        const out: (Row | Status)[] = [];
        const walk = (path: string, depth: number) => {
            const d = dirs[path];
            if (!d) return;
            if (d.state === 'loading') { if (path !== '') out.push({ path, depth, kind: 'loading' }); return; }
            if (d.state === 'error') { out.push({ path, depth, kind: 'error', error: d.error }); return; }
            if (d.items.length === 0 && path !== '') out.push({ path, depth, kind: 'empty' });
            d.items.forEach((node, i) => {
                out.push({ node, depth, set: d.items.length, pos: i + 1 });
                if (node.kind === 'dir' && expanded.includes(node.path)) walk(node.path, depth + 1);
            });
        };
        walk('', 1);
        return out;
    }, [dirs, expanded]);
    const nodes = flat.filter((r): r is Row => 'node' in r);
    const current = nodes.some((r) => r.node.path === focused) ? focused : nodes[0]?.node.path ?? '';

    const focusRow = (path: string) => { setFocused(path); rows.current.get(path)?.focus(); };
    const onKeyDown = (e: KeyboardEvent<HTMLElement>) => {
        // Keys pressed on a row's own buttons belong to the buttons.
        const target = e.target as HTMLElement;
        const path = target.getAttribute('role') === 'treeitem' ? target.dataset.path : undefined;
        if (path === undefined) return;
        const i = nodes.findIndex((r) => r.node.path === path);
        const node = nodes[i]?.node;
        if (!node) return;
        const open = node.kind === 'dir' && expanded.includes(path);
        let handled = true;
        switch (e.key) {
            case 'ArrowDown': if (nodes[i + 1]) focusRow(nodes[i + 1].node.path); break;
            case 'ArrowUp': if (nodes[i - 1]) focusRow(nodes[i - 1].node.path); break;
            case 'Home': focusRow(nodes[0].node.path); break;
            case 'End': focusRow(nodes[nodes.length - 1].node.path); break;
            case 'ArrowRight':
                if (node.kind === 'dir' && !open) toggle(path);
                else if (open && nodes[i + 1]?.depth === nodes[i].depth + 1) focusRow(nodes[i + 1].node.path);
                break;
            case 'ArrowLeft':
                if (open) toggle(path);
                else if (nodes[i].depth > 1) focusRow(parentOf(path));
                break;
            case 'Enter': case ' ':
                if (node.kind === 'dir') toggle(path); else onActivate?.(node);
                break;
            default: handled = false;
        }
        if (handled) e.preventDefault();
    };

    const rootDir = dirs[''];
    if (!rootDir || rootDir.state === 'loading') return <Box role="status" sx={{ display: 'flex', alignItems: 'center', gap: 1, p: 2, color: 'text.secondary' }}><CircularProgress size={16} /><Typography variant="body2">{t('tree.loading')}</Typography></Box>;
    if (rootDir.state === 'error') return <Alert severity="error" action={<Button color="inherit" onClick={() => retry('')}>{t('tree.retry')}</Button>}>{rootDir.error}</Alert>;
    if (rootDir.items.length === 0) return <>{empty ?? <Typography variant="body2" color="text.secondary" sx={{ py: 2 }}>{t('tree.emptyFolder')}</Typography>}</>;

    const pad = (depth: number) => 0.5 + (depth - 1) * 2;
    return (
        <Box role="tree" aria-label={ariaLabel} onKeyDown={onKeyDown}>
            {flat.map((r) => {
                if (!('node' in r)) {
                    // Progress or trouble inside an open folder, indented under it.
                    return (
                        <Box key={`${r.kind}:${r.path}`} role="none" sx={{ display: 'flex', alignItems: 'center', gap: 1, py: 0.5, pl: pad(r.depth) + 2.75, color: 'text.secondary' }}>
                            {r.kind === 'loading' && <><CircularProgress size={12} /><Typography variant="caption" role="status">{t('tree.loading')}</Typography></>}
                            {r.kind === 'empty' && <Typography variant="caption">{t('tree.emptyFolder')}</Typography>}
                            {r.kind === 'error' && <><Typography variant="caption" color="error">{r.error}</Typography><Button size="small" onClick={() => retry(r.path)}>{t('tree.retry')}</Button></>}
                        </Box>
                    );
                }
                const { node, depth, set, pos } = r;
                const isDir = node.kind === 'dir';
                const open = isDir && expanded.includes(node.path);
                const isCurrent = node.path === current;
                return (
                    <Box key={node.path} role="treeitem" aria-label={node.name} aria-level={depth} aria-setsize={set} aria-posinset={pos}
                        aria-expanded={isDir ? open : undefined} data-path={node.path} tabIndex={isCurrent ? 0 : -1}
                        ref={(el: HTMLElement | null) => { if (el) rows.current.set(node.path, el); else rows.current.delete(node.path); }}
                        onFocus={(e) => { if (e.target === e.currentTarget) setFocused(node.path); }}
                        onClick={() => { setFocused(node.path); if (isDir) toggle(node.path); else onActivate?.(node); }}
                        sx={{
                            display: 'flex', alignItems: 'center', gap: 0.75, minHeight: 34, pr: 1, pl: pad(depth), borderRadius: radius.card, cursor: isDir || onActivate ? 'pointer' : 'default', outline: 'none',
                            '&:hover': { bgcolor: 'action.hover' }, '&:focus-visible': { outline: 2, outlineStyle: 'solid', outlineColor: 'primary.main', outlineOffset: -2 },
                            // Invisible actions must not catch clicks; touch has no hover, so they stay shown there.
                            '& .act': { opacity: 0, pointerEvents: 'none' },
                            '&:focus .act, &:focus-within .act': { opacity: 1, pointerEvents: 'auto' },
                            '&:focus .meta, &:focus-within .meta': { opacity: 0 },
                            '@media (hover: hover)': { '&:hover .act': { opacity: 1, pointerEvents: 'auto' }, '&:hover .meta': { opacity: 0 } },
                            '@media (hover: none)': { '& .act': { opacity: 1, pointerEvents: 'auto' }, '& .meta': { opacity: 0 } },
                        }}>
                        <ChevronRight sx={{ fontSize: 16, color: 'text.disabled', visibility: isDir ? 'visible' : 'hidden', transform: open ? 'rotate(90deg)' : 'none', transition: 'transform 120ms' }} />
                        {isDir
                            ? (open ? <FolderOpen sx={{ fontSize: 18, color: 'primary.main' }} /> : <Folder sx={{ fontSize: 18, color: 'primary.main' }} />)
                            : <Description sx={{ fontSize: 18, color: 'text.secondary' }} />}
                        <Typography noWrap title={node.name} sx={{ flex: 1, minWidth: 0, fontSize: fontSizes.base, color: 'text.primary', fontFamily: fontMono }}>{node.name}</Typography>
                        {(renderMeta || renderActions) && (
                            // Meta and actions share one slot: the actions replace the meta while the row is hovered or focused.
                            <Box sx={{ position: 'relative', flexShrink: 0, minWidth: 100, height: 28, display: 'flex', alignItems: 'center', justifyContent: 'flex-end' }}>
                                {renderMeta && <Box className="meta" sx={{ color: 'text.secondary', fontSize: fontSizes.micro, whiteSpace: 'nowrap' }}>{renderMeta(node)}</Box>}
                                {renderActions && <Box className="act" onClick={(e) => e.stopPropagation()} sx={{ position: 'absolute', inset: 0, display: 'flex', justifyContent: 'flex-end', alignItems: 'center' }}>{renderActions(node, { tabIndex: isCurrent ? 0 : -1 })}</Box>}
                            </Box>
                        )}
                    </Box>
                );
            })}
        </Box>
    );
}
