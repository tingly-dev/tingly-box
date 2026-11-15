import { memo } from 'react';
import { Box } from '@mui/material';
import { alpha, type Theme } from '@mui/material/styles';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { RowKind, type ParsedPatch } from './diffModel';
import { useVirtualWindow } from './useVirtualWindow';

const ROW = 20;
const GUTTER_CH = 6;
// A minified line can be megabytes wide; the scroll width stops growing here.
const MAX_WIDTH_CH = 400;

// Rows are plain elements styled by the scroller's one `sx`: scrolling mounts
// dozens of rows a frame, and a styled component per cell made that the cost.
const patchSx = (t: Theme) => ({
    flex: 1, minHeight: 0, overflow: 'auto', fontFamily: fontMono, fontSize: fontSizes.sm,
    '& .r': { display: 'flex', height: ROW, lineHeight: `${ROW}px`, whiteSpace: 'pre' },
    '& .r.add': { backgroundColor: alpha(t.palette.success.main, 0.14) },
    '& .r.del': { backgroundColor: alpha(t.palette.error.main, 0.14) },
    '& .r.hunk': { backgroundColor: t.palette.action.hover, color: t.palette.text.secondary, paddingLeft: t.spacing(1) },
    '& .r.note': { color: t.palette.text.secondary, fontStyle: 'italic', paddingLeft: `${GUTTER_CH * 2 + 3}ch` },
    '& .n': { flexShrink: 0, width: `${GUTTER_CH}ch`, padding: t.spacing(0, 0.5), textAlign: 'right', color: t.palette.text.disabled, userSelect: 'none' },
    '& .del .n.o': { backgroundColor: alpha(t.palette.error.main, 0.24) },
    '& .add .n.w': { backgroundColor: alpha(t.palette.success.main, 0.24) },
    '& .s': { width: '2ch', flexShrink: 0, textAlign: 'center', color: t.palette.text.disabled, userSelect: 'none' },
    '& .add .s': { color: t.palette.success.main },
    '& .del .s': { color: t.palette.error.main },
});

const CLASS = ['hunk', 'add', 'del', 'ctx', 'note'] as const;

const Row = memo(function Row({ parsed, i }: { parsed: ParsedPatch; i: number }) {
    const k = parsed.kind[i];
    const text = parsed.text[i];
    if (k === RowKind.Hunk || k === RowKind.Note) return <div className={`r ${CLASS[k]}`}>{text}</div>;
    return (
        <div className={`r ${CLASS[k]}`}>
            <span className="n o">{parsed.oldNo[i] || ''}</span>
            <span className="n w">{parsed.newNo[i] || ''}</span>
            <span className="s">{k === RowKind.Add ? '+' : k === RowKind.Del ? '-' : ''}</span>
            <span>{text}</span>
        </div>
    );
});

/** One file's patch. Only the rows in view are in the DOM, so a 100k-line
 * patch scrolls like a short one. */
export function DiffPatchView({ parsed }: { parsed: ParsedPatch }) {
    const count = parsed.text.length;
    const { ref, onScroll, start, end, totalHeight } = useVirtualWindow(count, ROW);
    const rows = [];
    for (let i = start; i < end; i++) rows.push(<Row key={i} parsed={parsed} i={i} />);
    return (
        <Box ref={ref} onScroll={onScroll} data-testid="diff-patch" sx={patchSx}>
            <div style={{ position: 'relative', height: totalHeight, minWidth: `${GUTTER_CH * 2 + 3 + Math.min(parsed.maxLen, MAX_WIDTH_CH)}ch` }}>
                <div style={{ position: 'absolute', top: start * ROW, left: 0, right: 0 }}>{rows}</div>
            </div>
        </Box>
    );
}
