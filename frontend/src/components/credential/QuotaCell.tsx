import { Box, Button, Tooltip, Typography } from '@mui/material';
import { useState, type MouseEvent, type ReactNode } from 'react';
import { Code, Refresh } from '@/components/icons';
import { useTranslation } from 'react-i18next';
import type { ProviderQuota, QuotaWindow, QuotaWindowDisplayItem } from '@/types/quota';
import {
    formatQuotaAvailable,
    formatQuotaRemaining,
    isCountable,
    quotaRemainingPercent,
    quotaToWindows,
    tightestWindow,
} from '@/types/quota';
import { QUOTA_COLORS, formatNumber } from '../dashboard/chartStyles';
import { QuotaRing, formatQuotaDuration, quotaRingColor, quotaRingSpinSx } from './QuotaRing';
import { QuotaRawResponseDialog } from './QuotaRawResponseDialog';
import { useQuotaBars } from './useQuotaBars';
import { fontSizes } from '@/theme/fonts';

// Older than this, the figure is dimmed: the cache is refreshed in the
// background, so a stale snapshot means the refresher could not reach upstream.
const STALE_AFTER_MS = 60 * 60 * 1000;

// Lines shown in the cell: up to two allowances, plus one line kept for money
// (a balance, a wallet, a spend) when there is any — a balance is exactly the
// figure that must not end up behind "+N". Three caption lines still fit the
// row height the Actions group already sets.
const MAX_ALLOWANCE_LINES = 2;
const MAX_VALUE_LINES_ALONE = 2;
// Resource groups (e.g. Codex reset credits, "3/4") take whatever is left of
// the three lines, so they show up front when they fit instead of only in the hover.
const MAX_LINES = 3;

interface QuotaCellProps {
    quota: ProviderQuota | undefined;
    refreshing: boolean;
    onRefresh: () => void;
}

/** "5h" / "7d" / "30m" from a window's period; undefined when it has none. */
function periodLabel(minutes?: number): string | undefined {
    if (!minutes || minutes <= 0) return undefined;
    if (minutes % 1440 === 0) return `${minutes / 1440}d`;
    if (minutes % 60 === 0) return `${minutes / 60}h`;
    return `${minutes}m`;
}

/** Money is shown as an amount, never as a share: "81.41 CNY", "$37.50". */
function isMoney(window: QuotaWindow): boolean {
    return window.unit === 'currency';
}

function formatMoneyCode(value: number, currencyCode?: string): string {
    const amount = value.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    return currencyCode ? `${amount} ${currencyCode}` : `$${amount}`;
}

function formatMoney(value: number, window: QuotaWindow): string {
    return formatMoneyCode(value, window.currency_code);
}

interface CellLine {
    item: QuotaWindowDisplayItem;
    /** allowance: a share of a cap · wallet: money against a cap · balance: an amount left · spend: an amount used · resource: a group of credits/entries */
    kind: 'allowance' | 'wallet' | 'balance' | 'spend' | 'resource';
    /** Remaining share, for the ring; only allowance and wallet have one. */
    remaining?: number;
    text: string;
    /** Resource only: nothing left, shown as a muted dot. */
    exhausted?: boolean;
}

/**
 * The Quota column of the credential tables. One line per window worth a
 * figure — the same ring the rule graph shows on a service node, the window's
 * name, and the share left ("left", not a bare percent, which reads as either
 * used or remaining). Money is an amount, not a share: a balance ("81.41 CNY",
 * no ring), a capped wallet ("$37.50 left", with its ring), or an uncapped
 * spend ("$8.10 used", no ring — there is nothing to run out of).
 *
 * Allowances keep quotaToWindows' order (self-healing limits first, shorter
 * periods first), so a 5h + weekly plan reads "5h" then "7d"; money comes
 * after them on a line of its own. Anything left over is marked "+N". Every window, reset time, cost and
 * freshness is in the tooltip, along with Refresh and — when upstream sent
 * one — Details (the raw response). Clicking the cell also refreshes.
 *
 * No reading → "—", nothing more (.design/quota-semantics.md §3.6), but the
 * cell stays clickable so a reading can still be requested.
 */
export function QuotaCell({ quota, refreshing, onRefresh }: QuotaCellProps) {
    const { t } = useTranslation();
    const [rawOpen, setRawOpen] = useState(false);
    const { resourceItems } = useQuotaBars(quota);
    const windows = quotaToWindows(quota);
    const tightest = tightestWindow(quota);
    const describe = (item: QuotaWindowDisplayItem): CellLine | undefined => {
        const { window } = item;
        const countable = isCountable(window);
        if (isMoney(window)) {
            if (countable) {
                const left = Math.max(0, window.available ?? window.limit - window.used);
                return { item, kind: 'wallet', remaining: quotaRemainingPercent(window), text: t('rule.service.quota.left', { value: formatMoney(left, window) }) };
            }
            if (window.available != null) return { item, kind: 'balance', text: formatMoney(window.available, window) };
            if (window.used > 0) return { item, kind: 'spend', text: t('providerTable.quota.used', { value: formatMoney(window.used, window) }) };
            return undefined;
        }
        if (countable) {
            const remaining = quotaRemainingPercent(window);
            return { item, kind: 'allowance', remaining, text: t('rule.service.quota.left', { value: `${Math.round(remaining)}%` }) };
        }
        // A non-money balance, e.g. credits reported only as what is left.
        const available = formatQuotaAvailable(window, formatNumber);
        return available ? { item, kind: 'balance', text: available } : undefined;
    };
    const described = windows.map(describe).filter((line): line is CellLine => !!line);
    const allowances = described.filter(line => line.kind === 'allowance');
    const values = described.filter(line => line.kind !== 'allowance');
    const allowanceLines = allowances.slice(0, MAX_ALLOWANCE_LINES);
    const primary = [...allowanceLines, ...values.slice(0, allowanceLines.length ? 1 : MAX_VALUE_LINES_ALONE)];
    const resources: CellLine[] = resourceItems
        .filter(r => r.key === 'resource')
        .map(r => ({
            item: { key: `resource:${r.key}`, label: r.window.label ?? r.key, window: r.window },
            kind: 'resource' as const,
            exhausted: r.exhausted,
            text: r.countLabel,
        }));
    const lines = [...primary, ...resources.slice(0, Math.max(0, MAX_LINES - primary.length))];
    const hidden = described.length + resources.length - lines.length;

    // Allowances are named by period ("5h", "7d") when that tells them apart —
    // short and scannable down a column. Two of the same period (e.g.
    // per-model daily limits) need their own labels instead. Money reads as
    // "Balance" (the currency is in the figure), a spend by its period.
    const periods = allowanceLines.map(({ item }) => periodLabel(item.window.window_minutes));
    const periodsDistinct = periods.every(p => p) && new Set(periods).size === periods.length;
    const nameOf = (line: CellLine, i: number): string | undefined => {
        if (line.kind === 'allowance') return periodsDistinct ? periods[i] : line.item.label;
        if (line.kind === 'spend') return periodLabel(line.item.window.window_minutes) ?? line.item.label;
        return isMoney(line.item.window) ? t('providerTable.quota.balance') : line.item.label;
    };

    const now = Date.now();
    const fetchedAt = quota?.fetched_at ? new Date(quota.fetched_at).getTime() : NaN;
    const stale = Number.isFinite(fetchedAt) && now - fetchedAt > STALE_AFTER_MS;
    const lastError = quota?.last_error;
    const hasRaw = quota?.raw_response != null;

    // A cost that restates a money window (e.g. Kimi's booster wallet) would
    // show the same amount twice; it only earns a row when nothing else does.
    const hasMoneyLine = described.some(line => isMoney(line.item.window));
    const cost = quota?.cost && !hasMoneyLine ? quota.cost : undefined;
    const costText = cost && (cost.limit > 0
        ? t('providerTable.quota.leftOf', {
            value: formatMoneyCode(Math.max(0, cost.limit - cost.used), cost.currency_code),
            limit: formatMoneyCode(cost.limit, cost.currency_code),
        })
        : t('providerTable.quota.used', { value: formatMoneyCode(cost.used, cost.currency_code) }));

    const tooltip = (
        <Box sx={{ minWidth: 240, py: 0.25 }}>
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.75 }}>
                {windows.map((item) => {
                    const { key, label, window } = item;
                    const line = describe(item);
                    const value = isMoney(window)
                        ? line?.text
                        : isCountable(window)
                            ? t('rule.service.quota.left', { value: formatQuotaRemaining(window, formatNumber) })
                            : formatQuotaAvailable(window, formatNumber);
                    if (!value) return null;
                    const resetsAt = window.resets_at ? new Date(window.resets_at).getTime() : NaN;
                    return (
                        <TooltipRow
                            key={key}
                            remaining={line?.remaining}
                            label={label}
                            value={value}
                            detail={Number.isFinite(resetsAt) && resetsAt > now
                                ? t('rule.service.quota.resetsIn', { duration: formatQuotaDuration(resetsAt - now) })
                                : undefined}
                        />
                    );
                })}
                {resourceItems.map(item => (
                    <Box key={item.key} sx={{ display: 'flex', flexDirection: 'column', gap: 0.5 }}>
                        <TooltipRow dot dotMuted={item.exhausted} label={item.window.label ?? item.key} value={item.countLabel} />
                        {item.entries.map(({ key, label, window }) => {
                            const voucher = window.unit === 'credits' && window.limit === 1;
                            const resetsAt = window.resets_at ? new Date(window.resets_at).getTime() : NaN;
                            return (
                                <TooltipRow
                                    key={key}
                                    indent
                                    label={label}
                                    value={voucher
                                        ? window.label
                                        : isCountable(window)
                                            ? t('rule.service.quota.left', { value: formatQuotaRemaining(window, formatNumber) })
                                            : formatQuotaAvailable(window, formatNumber)}
                                    detail={voucher
                                        ? window.description
                                        : Number.isFinite(resetsAt) && resetsAt > now
                                            ? t('rule.service.quota.resetsIn', { duration: formatQuotaDuration(resetsAt - now) })
                                            : window.description}
                                />
                            );
                        })}
                    </Box>
                ))}
                {cost && costText && (
                    <TooltipRow label={cost.label || t('providerTable.quota.cost')} value={costText} />
                )}
                {described.length === 0 && (
                    <Typography variant="caption" sx={{ color: lastError ? QUOTA_COLORS.error : 'text.secondary' }}>
                        {lastError
                            ? t('providerTable.quota.readFailed')
                            : quota ? t('providerTable.quota.noLimits') : t('providerTable.quota.none')}
                    </Typography>
                )}
            </Box>
            {/* Actions live in the hover itself — a row menu is where nobody
                looks. React events bubble through the tooltip's portal, so each
                button stops propagation to keep row-level handlers out of it. */}
            <Box
                sx={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 1,
                    mt: 1,
                    pt: 0.75,
                    borderTop: '1px solid',
                    borderColor: 'divider',
                }}
            >
                <Typography variant="caption" sx={{ color: 'text.secondary', flex: 1, whiteSpace: 'nowrap' }}>
                    {refreshing
                        ? t('rule.service.quota.refreshing')
                        : Number.isFinite(fetchedAt) && t('rule.service.quota.updated', { duration: formatQuotaDuration(now - fetchedAt) })}
                </Typography>
                <Box sx={{ display: 'flex', gap: 0.25, mr: -0.75 }}>
                    <TooltipAction
                        icon={<Refresh sx={{ fontSize: 14 }} />}
                        label={t('providerTable.quota.refresh')}
                        disabled={refreshing}
                        onClick={onRefresh}
                    />
                    {hasRaw && (
                        <TooltipAction
                            icon={<Code sx={{ fontSize: 14 }} />}
                            label={t('providerTable.quota.rawResponse')}
                            onClick={() => setRawOpen(true)}
                        />
                    )}
                </Box>
            </Box>
        </Box>
    );

    let figure: ReactNode;
    if (lines.length > 0) {
        figure = (
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.125, minWidth: 0, opacity: refreshing ? 0.6 : 1 }}>
                {lines.map((line, i) => {
                    const { remaining } = line;
                    return (
                        <Box key={line.item.key} sx={{ display: 'flex', alignItems: 'center', gap: 0.75, minWidth: 0 }}>
                            {line.kind === 'resource' ? (
                                <ResourceDot size={14} muted={line.exhausted} />
                            ) : remaining != null ? (
                                <Box component="span" sx={{ display: 'inline-flex', ...(refreshing && quotaRingSpinSx) }}>
                                    {/* While refreshing, a fixed quarter arc spins like a loader — the
                                        real arc can be empty (used up), and an empty ring shows no motion. */}
                                    <QuotaRing remaining={refreshing ? 25 : remaining} color={quotaRingColor(remaining)} size={14} />
                                </Box>
                            ) : (
                                // Keeps ringless lines aligned with the ringed ones above/below.
                                <Box component="span" sx={{ width: 14, flexShrink: 0 }} />
                            )}
                            <Typography
                                variant="caption"
                                sx={{
                                    color: 'text.secondary',
                                    lineHeight: 1.4,
                                    flexShrink: 1,
                                    minWidth: 22,
                                    maxWidth: 64,
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                    whiteSpace: 'nowrap',
                                }}
                            >
                                {nameOf(line, i)}
                            </Typography>
                            <Typography
                                variant="caption"
                                sx={{ fontWeight: 600, lineHeight: 1.4, fontVariantNumeric: 'tabular-nums', whiteSpace: 'nowrap', color: 'text.primary' }}
                            >
                                {line.text}
                            </Typography>
                            {hidden > 0 && i === lines.length - 1 && (
                                <Typography variant="caption" sx={{ color: 'text.disabled', lineHeight: 1.4, whiteSpace: 'nowrap' }}>
                                    +{hidden}
                                </Typography>
                            )}
                        </Box>
                    );
                })}
            </Box>
        );
    } else if (refreshing) {
        figure = (
            <Box component="span" sx={{ display: 'inline-flex', ...quotaRingSpinSx }}>
                <QuotaRing remaining={25} color={QUOTA_COLORS.secondary} size={14} />
            </Box>
        );
    } else {
        figure = <Typography variant="body2" sx={{ color: 'text.disabled' }}>—</Typography>;
    }

    const tightestRemaining = tightest ? Math.round(quotaRemainingPercent(tightest)) : undefined;
    return (
        <>
        <Tooltip title={tooltip} arrow placement="top" slotProps={{ tooltip: { sx: { maxWidth: 380 } } }}>
            <Box
                component="span"
                role="button"
                tabIndex={0}
                aria-label={tightestRemaining != null
                    ? t('rule.service.quota.left', { value: `${tightestRemaining}%` })
                    : t('providerTable.quota.refresh')}
                aria-busy={refreshing}
                onClick={() => !refreshing && onRefresh()}
                onKeyDown={(e) => {
                    if (e.key !== 'Enter' && e.key !== ' ') return;
                    e.preventDefault();
                    if (!refreshing) onRefresh();
                }}
                sx={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    maxWidth: '100%',
                    minWidth: 0,
                    px: 0.75,
                    py: 0.25,
                    mx: -0.75,
                    borderRadius: 1,
                    opacity: stale && !refreshing ? 0.5 : 1,
                    cursor: refreshing ? 'progress' : 'pointer',
                    '&:hover': { bgcolor: 'action.hover' },
                    '&:focus-visible': { outline: '2px solid', outlineColor: 'primary.main' },
                }}
            >
                {figure}
            </Box>
        </Tooltip>
        <QuotaRawResponseDialog
            open={rawOpen}
            onClose={() => setRawOpen(false)}
            providerName={quota?.provider_name}
            response={quota?.raw_response}
        />
        </>
    );
}

/**
 * Marks a resource (credits, per-model entries) in the ring's slot. These are
 * counts of things, not shares of a cap, so a ring would imply a proportion —
 * a dot only says "there is one here" — green while any is left, muted once all are spent.
 */
function ResourceDot({ size, muted }: { size: number; muted?: boolean }) {
    return (
        <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', width: size, height: size, flexShrink: 0 }}>
            <Box component="span" sx={{ width: 6, height: 6, borderRadius: '50%', bgcolor: muted ? 'text.disabled' : QUOTA_COLORS.success }} />
        </Box>
    );
}

function TooltipAction({ icon, label, disabled, onClick }: {
    icon: ReactNode;
    label: string;
    disabled?: boolean;
    onClick: () => void;
}) {
    return (
        <Button
            size="small"
            variant="text"
            color="inherit"
            startIcon={icon}
            disabled={disabled}
            onClick={(e: MouseEvent) => {
                e.stopPropagation();
                onClick();
            }}
            sx={{
                minWidth: 0,
                px: 0.75,
                py: 0.25,
                fontSize: fontSizes.micro,
                fontWeight: 500,
                lineHeight: 1.4,
                color: 'text.secondary',
                textTransform: 'none',
                '&:hover': { color: 'primary.main', bgcolor: 'action.hover' },
                // MUI sizes a small button's start icon to 18px; match the caption text instead.
                '& .MuiButton-startIcon': { mr: 0.5, '& > *:nth-of-type(1)': { fontSize: 14 } },
            }}
        >
            {label}
        </Button>
    );
}

/**
 * One window in the hover: a small ring when the window has a share left,
 * its full name, and the figure right-aligned so a column of figures lines up;
 * the reset time sits underneath in the secondary color.
 */
function TooltipRow({ remaining, dot, dotMuted, label, value, detail, indent }: {
    remaining?: number;
    /** A dot instead of a ring: the row is an item or a group, not a share of a cap. */
    dot?: boolean;
    dotMuted?: boolean;
    /** A child entry of the group row above: shifted right, no ring slot of its own. */
    indent?: boolean;
    label: ReactNode;
    value: ReactNode;
    detail?: ReactNode;
}) {
    return (
        <Box sx={{ display: 'grid', gridTemplateColumns: '12px 1fr auto', columnGap: 1, alignItems: 'center', pl: indent ? 2.5 : 0 }}>
            <Box sx={{ display: 'inline-flex' }}>
                {dot
                    ? <ResourceDot size={12} muted={dotMuted} />
                    : remaining != null && <QuotaRing remaining={remaining} color={quotaRingColor(remaining)} size={12} />}
            </Box>
            <Typography variant="caption" sx={{ color: 'text.secondary', minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {label}
            </Typography>
            <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.primary', fontVariantNumeric: 'tabular-nums', whiteSpace: 'nowrap', textAlign: 'right', pl: 1.5 }}>
                {value}
            </Typography>
            {detail && (
                <Typography variant="caption" sx={{ gridColumn: '2 / 4', color: 'text.disabled', fontSize: fontSizes.xs, lineHeight: 1.3 }}>
                    {detail}
                </Typography>
            )}
        </Box>
    );
}
