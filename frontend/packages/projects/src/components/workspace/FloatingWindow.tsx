import { useEffect, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { Badge, Box, IconButton, Paper, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Close, FloatWindow, Minimize, OverviewPane as DockIcon } from '../icons';
import { readLocal, writeLocal } from '../../utils/storage';
import { radius } from '@tingly/ui/theme/radius';
import { fontSizes } from '@tingly/ui/theme/fonts';

export interface Rect { x: number; y: number; w: number; h: number }

export const WINDOW_MIN = { w: 360, h: 260 };
const TITLEBAR = 40;

/** Keep a window usable: at least the minimum size, and its title bar inside the viewport. */
export function clampRect(r: Rect, vw: number, vh: number): Rect {
    const w = Math.round(Math.min(Math.max(r.w, WINDOW_MIN.w), Math.max(WINDOW_MIN.w, vw - 16)));
    const h = Math.round(Math.min(Math.max(r.h, WINDOW_MIN.h), Math.max(WINDOW_MIN.h, vh - 16)));
    const x = Math.round(Math.min(Math.max(r.x, 8 - w + 120), vw - 120));
    const y = Math.round(Math.min(Math.max(r.y, 8), vh - TITLEBAR - 8));
    return { x, y, w, h };
}

function initialRect(key: string, def: { w: number; h: number }, clearRight: number): Rect {
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    try {
        const saved = JSON.parse(readLocal(key) ?? 'null') as Rect | null;
        if (saved && [saved.x, saved.y, saved.w, saved.h].every(Number.isFinite)) return clampRect(saved, vw, vh);
    } catch {
        // fall through to the default
    }
    return clampRect({ x: vw - clearRight - def.w - 32, y: 76, ...def }, vw, vh);
}

interface Props {
    open: boolean;
    title: ReactNode;
    /** Position and size are remembered per browser under this key. */
    storageKey: string;
    defaultSize?: { w: number; h: number };
    /** Width on the right edge a first-time window keeps free (a pane it should not cover). */
    clearRight?: number;
    /** Something in the window waits on the user: a dot on its minimised pill. */
    attention?: boolean;
    onClose: () => void;
    /** Put the content back in the side pane (or wherever it docks). */
    onDock?: () => void;
    dockLabel?: string;
    children: ReactNode;
}

/**
 * A window over the page: drag its title bar, resize from the corner,
 * minimise it to a pill in the corner. It does not take room from the page.
 */
const DEFAULT_SIZE = { w: 480, h: 600 };

/** Mounted only while open, so opening it again shows the window, not the pill. */
export function FloatingWindow(props: Props) {
    return props.open ? <Window {...props} /> : null;
}

function Window({ title, storageKey, defaultSize = DEFAULT_SIZE, clearRight = 0, attention, onClose, onDock, dockLabel, children }: Props) {
    const { t } = useTranslation('projects');
    const key = `${storageKey}.window`;
    const [rect, setRect] = useState<Rect>(() => initialRect(key, defaultSize, clearRight));
    const [minimized, setMinimized] = useState(false);
    const drag = useRef<{ kind: 'move' | 'size'; px: number; py: number; from: Rect } | null>(null);
    const [active, setActive] = useState(false);
    const last = useRef(rect);
    useEffect(() => {
        last.current = rect;
    }, [rect]);

    useEffect(() => {
        const fit = () => setRect((r) => clampRect(r, window.innerWidth, window.innerHeight));
        window.addEventListener('resize', fit);
        return () => window.removeEventListener('resize', fit);
    }, []);

    const begin = (kind: 'move' | 'size') => (e: React.PointerEvent<HTMLElement>) => {
        if (e.button !== 0 || (e.target as HTMLElement).closest('button')) return;
        e.preventDefault();
        e.currentTarget.setPointerCapture(e.pointerId);
        drag.current = { kind, px: e.clientX, py: e.clientY, from: rect };
        setActive(true);
    };
    const move = (e: React.PointerEvent) => {
        const d = drag.current;
        if (!d) return;
        const dx = e.clientX - d.px;
        const dy = e.clientY - d.py;
        const next = d.kind === 'move' ? { ...d.from, x: d.from.x + dx, y: d.from.y + dy } : { ...d.from, w: d.from.w + dx, h: d.from.h + dy };
        setRect(clampRect(next, window.innerWidth, window.innerHeight));
    };
    const end = () => {
        if (!drag.current) return;
        drag.current = null;
        setActive(false);
        writeLocal(key, JSON.stringify(last.current));
    };

    const node = minimized ? (
        <Paper
            role="button"
            tabIndex={0}
            elevation={4}
            aria-label={t('window.restore')}
            onClick={() => setMinimized(false)}
            onKeyDown={(e) => e.key === 'Enter' && setMinimized(false)}
            sx={{ position: 'fixed', right: 24, bottom: 24, zIndex: 1250, display: 'flex', alignItems: 'center', gap: 1, pl: 1.5, pr: 2, height: 36, maxWidth: 280, borderRadius: radius.pill, cursor: 'pointer', border: 1, borderColor: 'divider' }}
        >
            <Badge color="warning" variant="dot" invisible={!attention}><FloatWindow sx={{ fontSize: 16 }} /></Badge>
            <Typography noWrap sx={{ fontSize: fontSizes.base, fontWeight: 500 }}>{title}</Typography>
        </Paper>
    ) : (
        <Paper
            role="dialog"
            aria-label={typeof title === 'string' ? title : undefined}
            elevation={8}
            sx={{
                position: 'fixed', left: rect.x, top: rect.y, width: rect.w, height: rect.h, zIndex: 1250,
                display: 'flex', flexDirection: 'column', overflow: 'hidden', borderRadius: radius.card, border: 1, borderColor: 'divider',
                userSelect: active ? 'none' : undefined,
            }}
        >
            <Box
                onPointerDown={begin('move')} onPointerMove={move} onPointerUp={end} onPointerCancel={end}
                sx={{ display: 'flex', alignItems: 'center', gap: 0.5, height: TITLEBAR, flexShrink: 0, pl: 1.5, pr: 0.5, bgcolor: 'action.hover', borderBottom: 1, borderColor: 'divider', cursor: active ? 'grabbing' : 'grab', touchAction: 'none' }}
            >
                <Typography noWrap component="div" sx={{ flex: 1, minWidth: 0, fontSize: fontSizes.base, fontWeight: 650 }}>{title}</Typography>
                {onDock && (
                    <Tooltip title={dockLabel ?? t('window.dock')}>
                        <IconButton size="small" onClick={onDock} aria-label={dockLabel ?? t('window.dock')}><DockIcon fontSize="small" /></IconButton>
                    </Tooltip>
                )}
                <Tooltip title={t('window.minimize')}>
                    <IconButton size="small" onClick={() => setMinimized(true)} aria-label={t('window.minimize')}><Minimize fontSize="small" /></IconButton>
                </Tooltip>
                <Tooltip title={t('window.close')}>
                    <IconButton size="small" onClick={onClose} aria-label={t('window.close')}><Close fontSize="small" /></IconButton>
                </Tooltip>
            </Box>
            <Box sx={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column', bgcolor: 'background.paper' }}>{children}</Box>
            <Box
                onPointerDown={begin('size')} onPointerMove={move} onPointerUp={end} onPointerCancel={end}
                aria-hidden
                sx={{
                    position: 'absolute', right: 0, bottom: 0, width: 18, height: 18, cursor: 'nwse-resize', touchAction: 'none',
                    '&::after': { content: '""', position: 'absolute', right: 4, bottom: 4, width: 8, height: 8, borderRight: 2, borderBottom: 2, borderColor: 'text.disabled' },
                }}
            />
        </Paper>
    );
    return createPortal(node, document.body);
}
