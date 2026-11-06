import { useEffect, useRef, useState, type ReactNode, type RefObject } from 'react';
import { Box, Collapse, LinearProgress, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import type { Thread } from '../../client';
import { ChevronRight, ExpandMore } from '../icons';
import { ThreadStateMark } from './threadStatus';
import { formatTokens, timeAgo } from '../../utils/format';
import { cacheHitPct, contextFill, formatTurnDuration } from '../../utils/usage';
import { readLocal, writeLocal } from '../../utils/storage';
import { quotaTone, subscriptionRows } from '../../utils/quota';
import { serverNow } from '../../utils/serverClock';
import { useProjectsHost, type QuotaReading } from '../../host/ProjectsHost';

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
        <Box component="section" aria-label={title} sx={{ border: 1, borderColor: 'divider', borderRadius: radius.card, bgcolor: 'background.paper' }}>
            <Box role="button" tabIndex={0} aria-expanded={open} onClick={toggle} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggle(); } }}
                sx={{ display: 'flex', alignItems: 'center', gap: 0.5, px: 1.5, py: 1, cursor: 'pointer', userSelect: 'none' }}>
                <Typography sx={{ fontSize: fontSizes.sm, fontWeight: 600, color: 'text.primary' }}>{title}</Typography>
                {open ? <ExpandMore sx={{ fontSize: 16, color: 'text.secondary' }} /> : <ChevronRight sx={{ fontSize: 16, color: 'text.secondary' }} />}
                <Box sx={{ flex: 1 }} />
                {aside != null && <Typography component="span" sx={{ fontSize: fontSizes.sm, color: 'text.secondary' }}>{aside}</Typography>}
            </Box>
            <Collapse in={open}>
                <Box sx={{ px: 1.5, pb: 1.25 }}>{children}</Box>
            </Collapse>
        </Box>
    );
}

export function Fact({ label, title, children }: { label: string; title?: string; children: ReactNode }) {
    return (
        <Box sx={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 1.5, py: 0.25, fontSize: fontSizes.sm }}>
            <Box component="span" sx={{ color: 'text.secondary', flexShrink: 0 }}>{label}</Box>
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

/** Where a thread's launch is routed, for the host to look its quota up by. */
export interface QuotaRoute {
    profile: string;
    /** The model tier the launch asks for ("" for the default). */
    tier: string;
}

/** One quota window: what is left as a bar that shrinks as it is spent, and when it comes back. */
function QuotaBar({ label, text, usedPct, resetsAt, exhausted }: { label: string; text: string; usedPct?: number; resetsAt?: string; exhausted?: boolean }) {
    const { t, i18n } = useTranslation('projects');
    const tone = exhausted ? 'error' : usedPct == null ? 'primary' : quotaTone(usedPct);
    return (
        <Box sx={{ py: 0.25 }}>
            <Fact label={label} title={resetsAt ? t('rail.quotaResets', { when: timeAgo(resetsAt, i18n.language) }) : undefined}>{text}</Fact>
            {usedPct != null && (
                <LinearProgress variant="determinate" value={Math.max(0, 100 - usedPct)} color={tone}
                    aria-label={label} sx={{ height: 4, borderRadius: 2, mt: 0.25 }} />
            )}
        </Box>
    );
}

/**
 * What is left of the quota a thread draws on, the numbers Claude Code's
 * status line carries. Two sources, either may be missing: the subscription
 * windows (5-hour, weekly) the CLI read for a Claude.ai login, which the
 * thread carries, and the quota of the provider the host routes the thread to,
 * which the host's `quota` slot looks up. With neither there is no card.
 */
export function QuotaCard({ thread, route }: { thread: Thread; route?: QuotaRoute }) {
    const { t } = useTranslation('projects');
    const slot = useProjectsHost().session?.quota;
    const [reading, setReading] = useState<QuotaReading>();
    const profile = route?.profile;
    const tier = route?.tier;
    const model = thread.reported_model;
    useEffect(() => {
        if (!slot || profile == null || tier == null) return;
        let live = true;
        slot({ profile, tier, model })
            .then((r) => live && setReading(r))
            .catch(() => live && setReading(undefined));
        return () => {
            live = false;
        };
    }, [slot, profile, tier, model, thread.turns, thread.last_turn_at]);

    const rows = subscriptionRows(thread.rate_limits, serverNow());
    const windows = reading?.windows ?? [];
    if (rows.length === 0 && windows.length === 0) return null;
    const routed = reading?.provider && (reading.model ? `${reading.model} @ ${reading.provider}` : reading.provider);
    return (
        <RailCard id="quota" title={t('rail.quota')}>
            {rows.map((r) => (
                <QuotaBar key={r.key} label={t(`rail.quotaWindow.${r.key}`)} text={t('rail.quotaLeft', { pct: r.leftPct })} usedPct={r.usedPct} resetsAt={r.resetsAt} />
            ))}
            {windows.length > 0 && routed && <Fact label={t('rail.quotaProvider')} title={t('rail.quotaProviderTitle')}>{routed}</Fact>}
            {windows.map((w) => (
                <QuotaBar key={w.label} label={w.label} text={w.text} usedPct={w.usedPct} resetsAt={w.resetsAt} exhausted={w.limitReached} />
            ))}
        </RailCard>
    );
}

/** A session id is a long uuid: its first eight characters tell sessions apart, the rest is in the tooltip. */
export const shortSession = (id: string) => (id.length > 12 ? id.slice(0, 8) : id);
