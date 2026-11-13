import { useEffect, useLayoutEffect, useRef, useState, type ReactNode, type RefObject } from 'react';
import { Box, Collapse, LinearProgress, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import type { Thread } from '../../client';
import { Check } from '../icons';
import { DisclosureRow } from '../common/Disclosure';
import { useCopyFeedback } from '../../hooks/useCopyFeedback';
import { ThreadStateMark } from './threadStatus';
import { formatTokens, timeAgo } from '../../utils/format';
import { cacheHitPct, contextFill, formatTurnDuration } from '../../utils/usage';
import { readLocal, writeLocal } from '../../utils/storage';

/** How wide a thread panel must be to carry the rail beside the conversation. */
export const RAIL_MIN_PANEL = 880;
export const RAIL_WIDTH = 288;

const mono = { fontFamily: fontMono, fontSize: fontSizes.sm } as const;

/**
 * Whether the info rail sits beside a conversation: the box must be wide
 * enough, and the user can hide it (remembered per browser, one switch for the
 * coordinator and every thread). `watch` re-attaches the observer once the
 * box exists.
 */
export function useInfoRail(minPanel: number, watch: unknown): { ref: RefObject<HTMLDivElement | null>; wide: boolean; on: boolean; toggle: () => void } {
    const ref = useRef<HTMLDivElement>(null);
    const [wide, setWide] = useState(false);
    const [on, setOn] = useState(() => readLocal('thread.rail') !== '0');
    useEffect(() => {
        const el = ref.current;
        if (!el || typeof ResizeObserver === 'undefined') return;
        const ro = new ResizeObserver(([entry]) => setWide(entry.contentRect.width >= minPanel));
        ro.observe(el);
        return () => ro.disconnect();
    }, [minPanel, watch]);
    const toggle = () => {
        writeLocal('thread.rail', on ? '0' : '1');
        setOn(!on);
    };
    return { ref, wide, on, toggle };
}

/** The rail's frame beside a thread's conversation. */
export function InfoRailFrame({ label, children }: { label: string; children: ReactNode }) {
    return (
        <Box component="aside" aria-label={label} sx={{ width: RAIL_WIDTH, flexShrink: 0, borderLeft: 1, borderColor: 'divider', bgcolor: 'background.default', p: 1.5, display: 'flex', flexDirection: 'column', gap: 1.25, overflowY: 'auto', minHeight: 0 }}>
            {children}
        </Box>
    );
}

/** A card of the rail: a title the user can fold, remembered per browser. */
export function RailCard({ id, title, aside, defaultOpen = true, children }: { id: string; title: string; aside?: ReactNode; defaultOpen?: boolean; children: ReactNode }) {
    const key = `thread.rail.${id}`;
    const [open, setOpen] = useState(() => (readLocal(key) ?? (defaultOpen ? '1' : '0')) === '1');
    const toggle = () => {
        writeLocal(key, open ? '0' : '1');
        setOpen(!open);
    };
    return (
        <Box component="section" aria-label={title} sx={{ border: 1, borderColor: 'divider', borderRadius: radius.card, bgcolor: 'background.paper', overflow: 'hidden' }}>
            <DisclosureRow open={open} onToggle={toggle}>
                <Typography sx={{ fontSize: fontSizes.sm, fontWeight: 600, color: 'text.primary' }}>{title}</Typography>
                <Box sx={{ flex: 1 }} />
                {aside != null && <Typography component="span" sx={{ fontSize: fontSizes.sm, color: 'inherit' }}>{aside}</Typography>}
            </DisclosureRow>
            <Collapse in={open}>
                <Box sx={{ px: 1.5, pb: 1.25 }}>{children}</Box>
            </Collapse>
        </Box>
    );
}

/** The most of a value that fits the rail's value column by default; the real
 * budget is measured off the rendered row, this only seeds the first paint. */
const KEEP = 40;
const KEEP_MIN = 8;
const PROBE = '0000000000';

/**
 * The tail of a value too long for its column: the ellipsis leads, the end —
 * the part that tells values apart — is never cut. A path keeps whole
 * trailing directories (`…/worktrees/stop-28c4c8`), dropping earlier ones
 * until it fits; anything else keeps its last characters, stepping over a
 * separator near the cut so it doesn't start mid-word. Whatever comes back
 * fits `budget` characters (or is the value itself).
 */
export function keepTail(value: string, budget: number): string {
    if (value.length <= budget) return value;
    const segs = value.split('/').filter((s) => s.length > 0);
    // A path (no spaces) keeps its last one or two directories whole.
    if (segs.length > 1 && !value.includes(' ')) {
        for (const kept of [2, 1]) {
            const candidate = `…/${segs.slice(-kept).join('/')}`;
            if (candidate.length <= budget) return candidate;
        }
    } else {
        const tail = value.slice(-(budget - 1));
        const sep = tail.slice(0, 8).search(/ · | |:/);
        if (sep >= 0) return `…${tail.slice(sep + 1)}`;
    }
    // Even the last segment overflows: clip its head, the end stays whole.
    return `…${value.slice(-(budget - 1))}`;
}

/**
 * A Fact's value that stays on one line: a long value is cut by `keepTail`
 * against the width the row really has (never by CSS, so the end can't be
 * lost to a second, silent truncation), the tooltip tells the whole value on
 * hover, and a click copies it — a check marks the copy for a moment. Short
 * values show, hover and copy just the same.
 */
export function CopyValue({ value, copy }: { value: string; /** What a click copies (defaults to the value itself), as for a session id shown short. */ copy?: string }) {
    const { t } = useTranslation('projects');
    const { copied, copy: write } = useCopyFeedback();
    const full = copy ?? value;
    const textRef = useRef<HTMLSpanElement>(null);
    const probeRef = useRef<HTMLSpanElement>(null);
    const measuredFor = useRef('');
    const [budget, setBudget] = useState(KEEP);
    // How many characters the row really holds: measured once per value, from
    // the column the text fills when it overflows and the mono font's advance.
    useLayoutEffect(() => {
        const text = textRef.current;
        const probe = probeRef.current;
        if (!text || !probe) return;
        if (measuredFor.current !== value) {
            measuredFor.current = value;
            // A new value starts over; when the budget already sits at the
            // seed there is nothing to reset and the measure runs right away.
            if (budget !== KEEP) { setBudget(KEEP); return; }
        }
        if (text.scrollWidth <= text.clientWidth) return;
        // Fractions matter: a rounded advance can cost the tail a character.
        const unit = probe.getBoundingClientRect().width / PROBE.length;
        const avail = text.getBoundingClientRect().width;
        if (avail > 0 && unit > 0) setBudget((b) => Math.min(b, Math.max(KEEP_MIN, Math.floor(avail / unit))));
    }, [value, budget]);
    const shown = keepTail(value, budget);
    return (
        <Tooltip title={full} disableInteractive enterTouchDelay={0}>
            <Box component="span" role="button" tabIndex={0} aria-label={t('common.copy')}
                onClick={(e) => { e.stopPropagation(); void write(full); }}
                onKeyDown={(e) => { if (e.target === e.currentTarget && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); void write(full); } }}
                sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.25, minWidth: 0, maxWidth: '100%', cursor: 'copy', borderRadius: radius.row, '&:hover': { bgcolor: 'action.hover' } }}>
                <Box ref={textRef} component="span" sx={{ ...mono, overflow: 'hidden', whiteSpace: 'nowrap' }}>{shown}</Box>
                <Box ref={probeRef} component="span" aria-hidden sx={{ ...mono, position: 'absolute', visibility: 'hidden', whiteSpace: 'pre' }}>{PROBE}</Box>
                {copied && <Check sx={{ fontSize: 14, flexShrink: 0, color: 'primary.main' }} />}
            </Box>
        </Tooltip>
    );
}

/** A label and a value on one line. With `onClick` the whole line opens what
 * the value counts (the status area's running-tasks fact). With `labelWidth`
 * the labels take a fixed column, so every value in the card starts at the
 * same x instead of chasing the longest label above it. */
export function Fact({ label, title, labelWidth, onClick, children }: { label: string; title?: string; /** A fixed label column (px), as in the Environment card. */ labelWidth?: number; /** Opens what the value stands for; without it the line is plain text. */ onClick?: (e: { stopPropagation(): void; currentTarget: HTMLElement }) => void; children: ReactNode }) {
    return (
        <Box onClick={onClick}
            role={onClick ? 'button' : undefined} tabIndex={onClick ? 0 : undefined}
            onKeyDown={onClick ? (e) => { if (e.target === e.currentTarget && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); onClick(e); } } : undefined}
            sx={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: labelWidth != null ? 0.75 : 1.5, py: 0.25, fontSize: fontSizes.sm, ...(onClick && { cursor: 'pointer', '&:hover': { bgcolor: 'action.hover' }, borderRadius: radius.row }) }}>
            <Box component="span" sx={{ color: 'text.secondary', flexShrink: 0, ...(labelWidth != null && { width: labelWidth, minWidth: 'max-content' }) }}>{label}</Box>
            <Box component="span" title={title} sx={{ ...mono, color: 'text.primary', minWidth: 0, textAlign: 'right', overflowWrap: 'anywhere' }}>{children}</Box>
        </Box>
    );
}

/** How full the context window is: the numbers, and a bar once the window is known. */
export function ContextMeter({ thread }: { thread: Thread }) {
    const { t } = useTranslation('projects');
    const fill = contextFill(thread);
    if (!fill) return <Fact label={t('rail.context')}>{t('rail.contextEmpty')}</Fact>;
    const { used, window, pct } = fill;
    return (
        <Box sx={{ py: 0.25 }}>
            <Fact label={t('rail.context')} title={t('rail.contextTitle')}>
                {window > 0 ? `${formatTokens(used)} / ${formatTokens(window)} · ${pct}%` : formatTokens(used)}
            </Fact>
            {pct != null && (
                <LinearProgress variant="determinate" value={pct} color={pct >= 90 ? 'error' : pct >= 70 ? 'warning' : 'primary'}
                    aria-label={t('rail.context')} sx={{ height: 4, borderRadius: 2, mt: 0.25 }} />
            )}
        </Box>
    );
}

/** The turns a thread has run: how many, how long the last took, when it ended. */
export function TurnFacts({ thread }: { thread: Thread }) {
    const { t, i18n } = useTranslation('projects');
    const last = thread.last_turn_at ?? (thread.turns > 0 ? thread.updated_at : undefined);
    return (
        <>
            <Fact label={t('rail.turns')}>{thread.turns}</Fact>
            {thread.last_turn_ms > 0 && <Fact label={t('rail.lastTurn')}>{formatTurnDuration(thread.last_turn_ms)}</Fact>}
            {last && <Fact label={t('rail.lastActive')} title={new Date(last).toLocaleString()}>{timeAgo(last, i18n.language)}</Fact>}
        </>
    );
}

/**
 * How a thread stands: its state, how full its context is, its turns. The
 * coordinator's main thread, a project thread and a folder session all read
 * through this card; `children` are facts only one of them has (the coordinator's queue).
 */
export function StatusCard({ thread, children }: { thread: Thread; children?: ReactNode }) {
    const { t } = useTranslation('projects');
    return (
        <RailCard id="status" title={t('rail.status')}>
            <Fact label={t('rail.state')}>
                <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75 }}>
                    <ThreadStateMark status={thread.status} queued={thread.queued} />
                    {t(`thread.status.${thread.status === 'working' && thread.queued ? 'queued' : thread.status}`)}
                </Box>
            </Fact>
            {children}
            <ContextMeter thread={thread} />
            <TurnFacts thread={thread} />
        </RailCard>
    );
}

/** What a thread has spent: prompt and reply tokens, and how much of the prompt the cache served. */
export function UsageCard({ thread }: { thread: Thread }) {
    const { t } = useTranslation('projects');
    const total = thread.input_tokens + thread.output_tokens;
    if (total === 0) return null;
    const hit = cacheHitPct(thread);
    return (
        <RailCard id="usage" title={t('rail.usage')} aside={formatTokens(total)}>
            <Fact label={t('rail.tokens')}>{formatTokens(thread.input_tokens)} / {formatTokens(thread.output_tokens)}</Fact>
            {(thread.cache_read_tokens > 0 || thread.cache_write_tokens > 0) && (
                <>
                    <Fact label={t('rail.cache')} title={t('rail.cacheTitle')}>{formatTokens(thread.cache_read_tokens)} / {formatTokens(thread.cache_write_tokens)}</Fact>
                    {hit != null && <Fact label={t('rail.cacheHitLabel')} title={t('rail.cacheHitTitle')}>{hit}%</Fact>}
                </>
            )}
        </RailCard>
    );
}

/** A session id is a long uuid: its first eight characters tell sessions apart, the rest is in the tooltip. */
export const shortSession = (id: string) => (id.length > 12 ? id.slice(0, 8) : id);
