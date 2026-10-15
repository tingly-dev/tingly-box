import { useRef, useState } from 'react';
import { Box } from '@mui/material';
import { useTranslation } from 'react-i18next';

export const PANE_DEFAULT = 440;
export const PANE_MIN = 360;
/** The conversation keeps at least this much room beside the pane. */
const CONVERSATION_MIN = 480;

/** `room` is the width the conversation and the pane share. */
export function clampPane(width: number, room = window.innerWidth): number {
    const max = Math.max(PANE_MIN, room - CONVERSATION_MIN);
    return Math.round(Math.min(Math.max(width, PANE_MIN), max));
}

/**
 * The pane's left edge: drag it to resize, double-click to reset, or focus it
 * and use the arrow keys. `onCommit` runs once a drag or key press settles.
 */
export function PaneResizer({ width, onWidth, onCommit }: { width: number; onWidth: (w: number) => void; onCommit: (w: number) => void }) {
    const { t } = useTranslation();
    const start = useRef<{ x: number; width: number } | null>(null);
    const [dragging, setDragging] = useState(false);
    const last = useRef(width);
    last.current = width;

    const el = useRef<HTMLDivElement>(null);
    const set = (w: number) => {
        // The pane's parent row holds the conversation and the pane.
        const c = clampPane(w, el.current?.parentElement?.parentElement?.clientWidth);
        last.current = c;
        onWidth(c);
        return c;
    };

    return (
        <Box
            ref={el}
            role="separator"
            aria-orientation="vertical"
            aria-label={t('project.resizePane')}
            aria-valuenow={width}
            aria-valuemin={PANE_MIN}
            tabIndex={0}
            title={t('project.resizePane')}
            onPointerDown={(e) => {
                if (e.button !== 0) return;
                e.preventDefault();
                e.currentTarget.setPointerCapture(e.pointerId);
                start.current = { x: e.clientX, width };
                setDragging(true);
            }}
            onPointerMove={(e) => {
                if (!start.current) return;
                // The pane sits on the right, so dragging left widens it.
                set(start.current.width + start.current.x - e.clientX);
            }}
            onPointerUp={() => {
                if (!start.current) return;
                start.current = null;
                setDragging(false);
                onCommit(last.current);
            }}
            onDoubleClick={() => onCommit(set(PANE_DEFAULT))}
            onKeyDown={(e) => {
                const step = e.shiftKey ? 64 : 16;
                if (e.key === 'ArrowLeft') onCommit(set(width + step));
                else if (e.key === 'ArrowRight') onCommit(set(width - step));
                else return;
                e.preventDefault();
            }}
            sx={{
                position: 'absolute', top: 0, bottom: 0, left: -4, width: 8, zIndex: 2,
                cursor: 'col-resize', touchAction: 'none', outline: 'none',
                // A thin line that lights up on hover, focus and while dragging.
                '&::after': {
                    content: '""', position: 'absolute', top: 0, bottom: 0, left: 3, width: 2,
                    bgcolor: dragging ? 'primary.main' : 'transparent', transition: 'background-color .15s',
                },
                '&:hover::after, &:focus-visible::after': { bgcolor: 'primary.main' },
            }}
        >
            {/* While dragging, a full-window layer keeps the cursor and stops text selection under it. */}
            {dragging && <Box sx={{ position: 'fixed', inset: 0, cursor: 'col-resize', userSelect: 'none' }} />}
        </Box>
    );
}
