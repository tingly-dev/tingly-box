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

/** The top band's columns: by default they fit their content up to this tall (resetting returns to it), at least BAND_MIN, at most the room allows. */
export const BAND_DEFAULT = 192;
export const BAND_MIN = 96;
const BAND_MAX = 480;
/** The header, the band's own header and padding sit above its threads; the conversation keeps this much below. */
const BAND_CHROME = 52 + 36 + 16;
const BAND_CONVERSATION_MIN = 240;

/** `room` is the height of the page column the band and the conversation share (unmeasured, 0: the window). */
export function clampBand(height: number, room?: number): number {
    const max = Math.max(BAND_MIN, Math.min(BAND_MAX, (room || window.innerHeight) - BAND_CHROME - BAND_CONVERSATION_MIN));
    return Math.round(Math.min(Math.max(height, BAND_MIN), max));
}

interface EdgeProps {
    /** `left`: a pane's left edge (drag left to grow). `bottom`: a band's lower edge (drag down to grow). */
    edge: 'left' | 'bottom';
    value: number;
    min: number;
    /** Clamp to what the surrounding layout allows; `el` is the handle, to measure its parents. */
    clamp: (v: number, el: HTMLElement | null) => number;
    reset: number;
    label: string;
    onValue: (v: number) => void;
    /** Runs once a drag or key press settles. */
    onCommit: (v: number) => void;
}

/**
 * A drag handle on the edge of a surface that faces the content: drag to
 * resize, double-click to reset, or focus it and use the arrow keys.
 */
function EdgeResizer({ edge, value, min, clamp, reset, label, onValue, onCommit }: EdgeProps) {
    const horizontal = edge === 'left';
    const start = useRef<{ at: number; value: number } | null>(null);
    const [dragging, setDragging] = useState(false);
    const last = useRef(value);
    last.current = value;

    const el = useRef<HTMLDivElement>(null);
    const set = (v: number) => {
        const c = clamp(v, el.current);
        last.current = c;
        onValue(c);
        return c;
    };
    const at = (e: { clientX: number; clientY: number }) => (horizontal ? e.clientX : e.clientY);
    const [grow, shrink] = horizontal ? ['ArrowLeft', 'ArrowRight'] : ['ArrowDown', 'ArrowUp'];
    const cursor = horizontal ? 'col-resize' : 'row-resize';

    return (
        <Box
            ref={el}
            role="separator"
            aria-orientation={horizontal ? 'vertical' : 'horizontal'}
            aria-label={label}
            aria-valuenow={value}
            aria-valuemin={min}
            tabIndex={0}
            title={label}
            onPointerDown={(e) => {
                if (e.button !== 0) return;
                e.preventDefault();
                e.currentTarget.setPointerCapture(e.pointerId);
                start.current = { at: at(e), value };
                setDragging(true);
            }}
            onPointerMove={(e) => {
                if (!start.current) return;
                // A pane on the right grows leftwards; a band on top grows downwards.
                set(start.current.value + (horizontal ? start.current.at - at(e) : at(e) - start.current.at));
            }}
            onPointerUp={() => {
                if (!start.current) return;
                start.current = null;
                setDragging(false);
                onCommit(last.current);
            }}
            onDoubleClick={() => onCommit(set(reset))}
            onKeyDown={(e) => {
                const step = e.shiftKey ? 64 : 16;
                if (e.key === grow) onCommit(set(value + step));
                else if (e.key === shrink) onCommit(set(value - step));
                else return;
                e.preventDefault();
            }}
            sx={{
                position: 'absolute', zIndex: 2, cursor, touchAction: 'none', outline: 'none',
                ...(horizontal ? { top: 0, bottom: 0, left: -4, width: 8 } : { left: 0, right: 0, bottom: -4, height: 8 }),
                // A thin line that lights up on hover, focus and while dragging.
                '&::after': {
                    content: '""', position: 'absolute', bgcolor: dragging ? 'primary.main' : 'transparent', transition: 'background-color .15s',
                    ...(horizontal ? { top: 0, bottom: 0, left: 3, width: 2 } : { left: 0, right: 0, top: 3, height: 2 }),
                },
                '&:hover::after, &:focus-visible::after': { bgcolor: 'primary.main' },
            }}
        >
            {/* While dragging, a full-window layer keeps the cursor and stops text selection under it. */}
            {dragging && <Box sx={{ position: 'fixed', inset: 0, cursor, userSelect: 'none' }} />}
        </Box>
    );
}

/** The side pane's left edge. */
export function PaneResizer({ width, onWidth, onCommit }: { width: number; onWidth: (w: number) => void; onCommit: (w: number) => void }) {
    const { t } = useTranslation('projects');
    return (
        <EdgeResizer
            edge="left" value={width} min={PANE_MIN} reset={PANE_DEFAULT} label={t('project.resizePane')}
            // The pane's parent row holds the conversation and the pane.
            clamp={(w, el) => clampPane(w, el?.parentElement?.parentElement?.clientWidth)}
            onValue={onWidth} onCommit={onCommit}
        />
    );
}

/** The top band's lower edge, facing the conversation. */
export function BandResizer({ height, onHeight, onCommit }: { height: number; onHeight: (h: number) => void; onCommit: (h: number) => void }) {
    const { t } = useTranslation('projects');
    return (
        <EdgeResizer
            edge="bottom" value={height} min={BAND_MIN} reset={BAND_DEFAULT} label={t('project.resizeBand')}
            // The band's parent is the page column it shares with the conversation.
            clamp={(h, el) => clampBand(h, el?.parentElement?.parentElement?.clientHeight)}
            onValue={onHeight} onCommit={onCommit}
        />
    );
}
