import { memo, useEffect } from 'react';
import { Box } from '@mui/material';
import { alpha, type Theme } from '@mui/material/styles';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { useTranslation } from 'react-i18next';
import type { DiffFile } from '../../../client';
import { useVirtualWindow } from './useVirtualWindow';

export const FILE_ROW = 34;

const STATUS_LETTER: Record<string, string> = { added: 'A', deleted: 'D', renamed: 'R', copied: 'C', typechange: 'T', modified: 'M' };

export function splitPath(path: string): { dir: string; name: string } {
    const i = path.lastIndexOf('/');
    return i < 0 ? { dir: '', name: path } : { dir: path.slice(0, i + 1), name: path.slice(i + 1) };
}

// Rows are plain elements styled by the scroller's one `sx`, so scrolling a
// long list mounts cheap markup instead of a MUI component tree per row.
const listSx = (t: Theme) => ({
    flex: 1, minHeight: 0, overflowY: 'auto', overflowX: 'hidden',
    '& .f': { display: 'flex', alignItems: 'center', gap: t.spacing(0.75), height: FILE_ROW, padding: t.spacing(0, 1, 0, 1.25), cursor: 'pointer', minWidth: 0 },
    '& .f:hover': { backgroundColor: t.palette.action.hover },
    '& .f.sel, & .f.sel:hover': { backgroundColor: t.palette.action.selected },
    '& .f.seen:not(.sel)': { opacity: 0.55 },
    '& .f input': { flexShrink: 0, margin: 0, cursor: 'pointer', accentColor: t.palette.primary.main },
    '& .st': { width: '1.5ch', flexShrink: 0, fontFamily: fontMono, fontSize: fontSizes.sm, fontWeight: 600, color: t.palette.text.secondary },
    '& .st.added': { color: t.palette.success.main },
    '& .st.deleted': { color: t.palette.error.main },
    '& .st.renamed, & .st.copied': { color: t.palette.info.main },
    '& .st.typechange': { color: t.palette.warning.main },
    '& .nm': { flex: 1, minWidth: 0, display: 'flex', alignItems: 'baseline', gap: t.spacing(0.75), whiteSpace: 'nowrap' },
    '& .base': { flexShrink: 0, maxWidth: '70%', overflow: 'hidden', textOverflow: 'ellipsis', fontSize: fontSizes.md, fontWeight: 500, color: t.palette.text.primary },
    '& .gen .base': { color: t.palette.text.secondary },
    '& .dir': { minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', fontSize: fontSizes.sm, color: t.palette.text.secondary },
    '& .cnt': { flexShrink: 0, fontFamily: fontMono, fontSize: fontSizes.micro, color: t.palette.text.secondary, whiteSpace: 'nowrap' },
    '& .cnt .a': { color: t.palette.success.main },
    '& .cnt .d': { color: t.palette.error.main },
    '& .f.sel': { boxShadow: `inset 2px 0 0 ${alpha(t.palette.primary.main, 0.9)}` },
});

const FileRow = memo(function FileRow({ file, selected, viewed, onSelect, onToggleViewed }: {
    file: DiffFile; selected: boolean; viewed: boolean; onSelect: (path: string) => void; onToggleViewed: (file: DiffFile) => void;
}) {
    const { t } = useTranslation('projects');
    const { dir, name } = splitPath(file.path);
    return (
        <div
            role="option"
            aria-selected={selected}
            className={`f${selected ? ' sel' : ''}${viewed ? ' seen' : ''}${file.generated ? ' gen' : ''}`}
            title={file.old_path ? `${file.old_path} → ${file.path}` : file.path}
            onClick={() => onSelect(file.path)}
        >
            <input type="checkbox" checked={viewed} aria-label={t('thread.diff.viewed')} onClick={(e) => e.stopPropagation()} onChange={() => onToggleViewed(file)} />
            <span className={`st ${file.status}`} title={t(`thread.diff.status.${file.status}`, { defaultValue: file.status })}>{STATUS_LETTER[file.status] ?? 'M'}</span>
            <span className="nm">
                <span className="base">{name}</span>
                {dir && <span className="dir">{dir}</span>}
            </span>
            <span className="cnt">
                {file.binary ? 'bin' : <><span className="a">+{file.additions}</span> <span className="d">−{file.deletions}</span></>}
            </span>
        </div>
    );
});

/** The changed files, windowed: thousands of rows cost as much as a screenful. */
export function DiffFileList({ files, indexes, selectedPath, viewedSet, onSelect, onToggleViewed }: {
    files: DiffFile[]; indexes: number[]; selectedPath: string | null; viewedSet: Set<string>;
    onSelect: (path: string) => void; onToggleViewed: (file: DiffFile) => void;
}) {
    const { t } = useTranslation('projects');
    const { ref, onScroll, start, end, totalHeight, reveal } = useVirtualWindow(indexes.length, FILE_ROW);
    useEffect(() => {
        const pos = selectedPath == null ? -1 : indexes.findIndex((i) => files[i].path === selectedPath);
        if (pos >= 0) reveal(pos);
    }, [selectedPath, indexes, files, reveal]);
    const rows = [];
    for (let pos = start; pos < end; pos++) {
        const file = files[indexes[pos]];
        rows.push(<FileRow key={file.path} file={file} selected={file.path === selectedPath} viewed={viewedSet.has(file.path)} onSelect={onSelect} onToggleViewed={onToggleViewed} />);
    }
    return (
        <Box ref={ref} onScroll={onScroll} role="listbox" aria-label={t('thread.files', { count: indexes.length })} sx={listSx}>
            <div style={{ position: 'relative', height: totalHeight }}>
                <div style={{ position: 'absolute', top: start * FILE_ROW, left: 0, right: 0 }}>{rows}</div>
            </div>
        </Box>
    );
}
