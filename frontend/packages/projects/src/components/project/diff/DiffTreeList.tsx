import { memo, useCallback, useEffect, useId, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { Box } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { DiffFile } from '../../../client';
import { DisclosureChevron } from '../../common/Disclosure';
import { DiffIcon, FileAdded, FileDeleted, FileModified, FileRenamed, Folder, FolderOpen } from '../../icons';
import { FILE_ROW, listSx } from './DiffFileList';
import { ancestorsOf, flattenTree, treeStats, type DiffTree, type TreeNode, type TreeStats } from './diffTree';
import { useVirtualWindow } from './useVirtualWindow';

/** Up to this many files every folder starts open (the whole change is a screen or two); a bigger change opens its top-level folders only, so the tree is an outline to dig into. */
export const OPEN_ALL_UP_TO = 150;
const INDENT = 12;

const STATUS_ICON = { added: FileAdded, deleted: FileDeleted, renamed: FileRenamed, copied: FileRenamed, modified: FileModified, typechange: DiffIcon } as const;

const treeSx = (t: Parameters<typeof listSx>[0]) => ({
    ...listSx(t),
    outline: 'none',
    '&:focus-visible .cur': { boxShadow: `inset 0 0 0 2px ${t.palette.primary.main}` },
    '& .t .sp': { width: 16, flexShrink: 0 },
    '& .t .st': { display: 'flex', width: 16, justifyContent: 'center' },
    '& .t svg': { fontSize: 16 },
    '& .t.dir .nm .base': { maxWidth: '100%' },
    '& .t.dir svg': { color: t.palette.text.secondary },
    '& .cnt .p': { color: t.palette.text.secondary, marginRight: t.spacing(0.75) },
});

interface RowProps {
    index: number;
    node: TreeNode;
    file: DiffFile | null;
    open: boolean;
    selected: boolean;
    cursor: boolean;
    viewed: boolean;
    files: number;
    viewedFiles: number;
    additions: number;
    deletions: number;
    domId: string;
    onActivate: (index: number) => void;
    onToggleViewed: (file: DiffFile) => void;
}

const TreeRow = memo(function TreeRow(p: RowProps) {
    const { t } = useTranslation('projects');
    const { node, file } = p;
    const style = { paddingLeft: 10 + node.depth * INDENT };
    if (!file) {
        const all = p.viewedFiles === p.files;
        return (
            <div
                id={p.domId}
                role="treeitem"
                aria-level={node.depth + 1}
                aria-expanded={p.open}
                className={`f t dir${p.cursor ? ' cur' : ''}${all ? ' seen' : ''}`}
                style={style}
                title={node.id.slice(0, -1)}
                onClick={() => p.onActivate(p.index)}
            >
                <DisclosureChevron open={p.open} />
                {p.open ? <FolderOpen aria-hidden /> : <Folder aria-hidden />}
                <span className="nm"><span className="base">{node.name}</span></span>
                <span className="cnt">
                    <span className="p" title={t('thread.diff.folderViewed', { viewed: p.viewedFiles, total: p.files })}>{p.viewedFiles}/{p.files}</span>
                    <span className="a">+{p.additions}</span> <span className="d">−{p.deletions}</span>
                </span>
            </div>
        );
    }
    const Icon = STATUS_ICON[file.status as keyof typeof STATUS_ICON] ?? DiffIcon;
    return (
        <div
            id={p.domId}
            role="treeitem"
            aria-level={node.depth + 1}
            aria-selected={p.selected}
            className={`f t${p.selected ? ' sel' : ''}${p.cursor ? ' cur' : ''}${p.viewed ? ' seen' : ''}${file.generated ? ' gen' : ''}`}
            style={style}
            title={file.old_path ? `${file.old_path} → ${file.path}` : file.path}
            onClick={() => p.onActivate(p.index)}
        >
            <span className="sp" />
            <input type="checkbox" checked={p.viewed} aria-label={t('thread.diff.viewed')} onClick={(e) => e.stopPropagation()} onChange={() => p.onToggleViewed(file)} />
            <span className={`st ${file.status}`} title={t(`thread.diff.status.${file.status}`, { defaultValue: file.status })}><Icon aria-hidden /></span>
            <span className="nm"><span className="base">{node.name}</span></span>
            <span className="cnt">
                {file.binary ? 'bin' : <><span className="a">+{file.additions}</span> <span className="d">−{file.deletions}</span></>}
            </span>
        </div>
    );
});

/** The changed files as a folder tree (ARIA tree, one tab stop): ↑/↓ move, ←
 * folds a folder or goes to its parent, → opens it or goes to its first child,
 * Enter/Space toggles a folder or opens a file. Only the visible rows are drawn. */
export function DiffTreeList({ files, tree, shown, selectedPath, viewedSet, filtering, onSelect, onToggleViewed }: {
    files: DiffFile[]; tree: DiffTree;
    /** The files that pass the filters, as indexes into `files`. */
    shown: number[];
    selectedPath: string | null; viewedSet: Set<string>;
    /** A path filter is typed: its matches are shown unfolded. */
    filtering: boolean;
    onSelect: (path: string) => void; onToggleViewed: (file: DiffFile) => void;
}) {
    const { t } = useTranslation('projects');
    const uid = useId();
    const { nodes } = tree;
    // Folders the user opened or closed; the rest follow the default.
    const [open, setOpen] = useState<Map<string, boolean>>(() => new Map());
    const [cursor, setCursor] = useState<string | null>(null);
    const [seenSelection, setSeenSelection] = useState<string | null>(null);

    const defaultOpen = useCallback((n: TreeNode) => filtering || files.length <= OPEN_ALL_UP_TO || n.depth === 0, [filtering, files.length]);
    const isOpen = useCallback((n: TreeNode) => open.get(n.id) ?? defaultOpen(n), [open, defaultOpen]);

    // A newly selected file (a click, j/k, the first file on opening) takes the
    // cursor and unfolds the folders above it. Done while rendering, so the rows
    // below are already right and there is no flash of the folded tree.
    if (selectedPath !== seenSelection) {
        setSeenSelection(selectedPath);
        if (selectedPath != null) {
            setCursor(selectedPath);
            const closed = ancestorsOf(tree, selectedPath).filter((a) => !isOpen(nodes[a]));
            if (closed.length) setOpen((m) => { const next = new Map(m); for (const a of closed) next.set(nodes[a].id, true); return next; });
        }
    }

    const stats: TreeStats = useMemo(() => treeStats(tree, files, shown, viewedSet), [tree, files, shown, viewedSet]);
    const rows = useMemo(() => flattenTree(tree, stats, isOpen), [tree, stats, isOpen]);
    const { ref, onScroll, start, end, totalHeight, reveal } = useVirtualWindow(rows.length, FILE_ROW);

    const cursorNode = cursor == null ? -1 : (tree.byId.get(cursor) ?? -1);
    const cursorPos = cursorNode < 0 ? -1 : rows.indexOf(cursorNode);
    // Scroll to the cursor when it moves, not whenever the rows change (folding a folder must not yank the view back).
    const revealed = useRef<string | null>(null);
    useEffect(() => {
        if (cursor == null || revealed.current === cursor || cursorPos < 0) return;
        revealed.current = cursor;
        reveal(cursorPos);
    }, [cursor, cursorPos, reveal]);

    const toggleFolder = useCallback((n: TreeNode, to?: boolean) => {
        setOpen((m) => new Map(m).set(n.id, to ?? !(m.get(n.id) ?? defaultOpen(n))));
    }, [defaultOpen]);

    const onActivate = useCallback((index: number) => {
        const n = nodes[index];
        setCursor(n.id);
        if (n.file >= 0) onSelect(files[n.file].path);
        else toggleFolder(n);
    }, [nodes, files, onSelect, toggleFolder]);

    const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
        if (e.target !== e.currentTarget || e.metaKey || e.ctrlKey || e.altKey || rows.length === 0) return;
        const goto = (pos: number) => {
            const n = nodes[rows[Math.max(0, Math.min(rows.length - 1, pos))]];
            setCursor(n.id);
            if (n.file >= 0) onSelect(files[n.file].path);
        };
        const node = cursorPos >= 0 ? nodes[rows[cursorPos]] : null;
        const isDir = node != null && node.file < 0;
        switch (e.key) {
            case 'ArrowDown': goto(cursorPos + 1); break;
            case 'ArrowUp': goto(cursorPos < 0 ? 0 : cursorPos - 1); break;
            case 'Home': goto(0); break;
            case 'End': goto(rows.length - 1); break;
            case 'ArrowRight':
                if (!isDir) return;
                if (!isOpen(node)) toggleFolder(node, true);
                else if (cursorPos + 1 < rows.length && nodes[rows[cursorPos + 1]].parent === rows[cursorPos]) goto(cursorPos + 1);
                break;
            case 'ArrowLeft':
                if (!node) return;
                if (isDir && isOpen(node)) toggleFolder(node, false);
                else if (node.parent >= 0) setCursor(nodes[node.parent].id);
                break;
            case 'Enter':
            case ' ':
                if (!node) return;
                onActivate(rows[cursorPos]);
                break;
            default: return;
        }
        e.preventDefault();
    };

    const out = [];
    for (let pos = start; pos < end; pos++) {
        const index = rows[pos];
        const node = nodes[index];
        const file = node.file >= 0 ? files[node.file] : null;
        out.push(
            <TreeRow
                key={node.id}
                index={index}
                node={node}
                file={file}
                open={file ? false : isOpen(node)}
                selected={file != null && file.path === selectedPath}
                cursor={index === cursorNode}
                viewed={file != null && viewedSet.has(file.path)}
                files={stats.files[index]}
                viewedFiles={stats.viewed[index]}
                additions={stats.additions[index]}
                deletions={stats.deletions[index]}
                domId={`${uid}-${index}`}
                onActivate={onActivate}
                onToggleViewed={onToggleViewed}
            />,
        );
    }
    return (
        <Box
            ref={ref}
            onScroll={onScroll}
            onKeyDown={onKeyDown}
            role="tree"
            tabIndex={0}
            aria-label={t('thread.files', { count: shown.length })}
            aria-activedescendant={cursorNode >= 0 && cursorPos >= start && cursorPos < end ? `${uid}-${cursorNode}` : undefined}
            sx={treeSx}
        >
            <div style={{ position: 'relative', height: totalHeight }}>
                <div style={{ position: 'absolute', top: start * FILE_ROW, left: 0, right: 0 }}>{out}</div>
            </div>
        </Box>
    );
}
