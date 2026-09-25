// Re-export quota-related types from codegen
import type {
    UsageWindow,
    UsageCost,
    UsageAccount,
    UsageBreakdown,
    ProviderUsage,
} from '@/client';

/**
 * A quota window, with `kind` narrowed from the generated UsageWindow's bare
 * `string` to the two real values the backend ever sends — see
 * .design/quota-semantics.md.
 */
export type QuotaWindow = Omit<UsageWindow, 'kind'> & {
    kind?: 'limit' | 'resource';
};

/** The fields that decide whether a window has a figure to show. */
type CountableFields = Pick<QuotaWindow, 'limit' | 'unknown' | 'unlimited'>;

/**
 * Whether a window carries a usage figure worth comparing. Unknown means the
 * provider never reported one, unlimited means there is nothing to use up, and
 * without a cap there is nothing to measure against — none of the three is a
 * usage of 0%, and rendering one as a bar reads as an untouched allowance.
 */
export function isCountable(window: CountableFields): boolean {
    return !window.unknown && !window.unlimited && window.limit > 0;
}

/**
 * Rank, then period length — the same order the backend establishes.
 *
 * Rank 0 (self-healing allowances) requires an explicit `kind === 'limit'` —
 * an untagged window (`kind` undefined) is not assumed to belong there, so
 * it sorts alongside resources (rank 1) instead of jumping the queue. No
 * default toward "limit" here, mirroring ai/quota's windowRank in
 * semantic.go: only a positively-classified window gets the front seat.
 */
function windowSortKey(window: QuotaWindow): [number, number] {
    const rank = !isCountable(window) ? 2 : window.kind === 'limit' ? 0 : 1;
    const minutes = window.window_minutes && window.window_minutes > 0
        ? window.window_minutes
        : Number.MAX_SAFE_INTEGER;
    return [rank, minutes];
}

/** Used share of a countable window — mirrors ai/quota's UsageWindow.Percent. */
function usedPercent(window: QuotaWindow): number {
    if (window.used_percent) return window.used_percent;
    return window.used >= window.limit ? 100 : window.used / window.limit * 100;
}

/** A window with no known duration sorts last, as in ai/quota's periodRank. */
function periodRank(window: QuotaWindow): number {
    return window.window_minutes && window.window_minutes > 0
        ? window.window_minutes
        : Number.MAX_SAFE_INTEGER;
}

/**
 * The window that binds the next request — the most used countable one, ties
 * going to the shorter period. Mirrors ai/quota's Tightest() with no kind
 * filter (the display question); see .design/quota-semantics.md §3.3.
 */
export function tightestWindow(quota?: ProviderQuota): QuotaWindow | undefined {
    let best: QuotaWindow | undefined;
    for (const window of quota?.windows ?? []) {
        if (!isCountable(window)) continue;
        if (!best) {
            best = window;
            continue;
        }
        const pw = usedPercent(window);
        const pb = usedPercent(best);
        if (pw > pb || (pw === pb && periodRank(window) < periodRank(best))) {
            best = window;
        }
    }
    return best;
}

// Type aliases for convenience and backward compatibility.
// Omit + re-add `windows` rather than a plain intersection: ProviderUsage
// already declares `windows?: UsageWindow[]`, and TS does not merge two
// array-typed properties element-wise across an intersection — the wider
// UsageWindow[] branch can end up winning at use sites, silently losing the
// QuotaWindow narrowing.
export type ProviderQuota = Omit<ProviderUsage, 'windows'> & {
    windows?: QuotaWindow[];
};

// Re-export for consumers
export type { UsageWindow, UsageCost, UsageAccount, UsageBreakdown, ProviderUsage };

// Quota types for provider usage/limit information
// Note: Most types are now from codegen, see ../client/index.ts

export interface QuotaWindowDisplayItem {
    key: string;
    label: string;
    window: QuotaWindow;
}

export function quotaToWindows(quota?: ProviderQuota): QuotaWindowDisplayItem[] {
    if (!quota || !quota.windows?.length) return [];

    return quota.windows
        .map((window, index) => {
            const key = window.key || `window-${index}`;
            return { key, label: window.label || key, window };
        })
        .sort((a, b) => {
            const [ra, ma] = windowSortKey(a.window);
            const [rb, mb] = windowSortKey(b.window);
            return ra !== rb ? ra - rb : ma - mb;
        });
}

// Extended quota with breakdowns flattened for UI consumption
export interface QuotaDisplayItem {
    key: string;           // Unique identifier (e.g., model name or "aggregate")
    label: string;         // Display label
    group?: string;        // Group type ("model", "type", or undefined for aggregate)
    windows: UsageWindow[];
}

// Helper to convert ProviderQuota to display items
export function quotaToDisplayItems(quota: ProviderQuota): QuotaDisplayItem[] {
    const items: QuotaDisplayItem[] = [];

    // Add breakdowns first (per-model or per-type)
    if (quota.breakdowns && quota.breakdowns.length > 0) {
        for (const bd of quota.breakdowns) {
            items.push({
                key: bd.key,
                label: bd.label,
                group: bd.group,
                windows: bd.windows,
            });
        }
    }

    items.push({
        key: 'aggregate',
        label: 'Total',
        windows: quotaToWindows(quota).map(item => item.window),
    });

    return items;
}

interface FormatQuotaUsageOptions {
    formatNumber?: (value: number) => string;
}

type QuotaUsageValues = CountableFields & Pick<QuotaWindow, 'used' | 'used_percent' | 'unit' | 'available' | 'currency_code'>;

export function quotaRemainingPercent(window: QuotaUsageValues): number {
    if (!isCountable(window)) return 0;
    const percent = window.available == null
        ? 100 - window.used_percent
        : window.available / window.limit * 100;
    return Math.max(0, Math.min(100, percent));
}

export function formatQuotaRemaining(
    window: QuotaUsageValues,
    formatNumber: (value: number) => string = String
): string {
    if (!isCountable(window)) {
        return formatQuotaAvailable(window, formatNumber) ?? (window.unknown ? 'not reported' : 'no limit');
    }
    const remaining = Math.max(0, window.available ?? window.limit - window.used);
    if (window.unit === 'percent') return `${formatNumber(remaining)}%`;
    return `${formatNumber(remaining)} / ${formatNumber(window.limit)} ${window.currency_code || window.unit}`;
}

export function formatQuotaAvailable(
    window: Pick<QuotaWindow, 'available' | 'currency_code' | 'unit'>,
    formatNumber: (value: number) => string = String
): string | undefined {
    if (window.available == null) return undefined;

    const unit = window.currency_code || window.unit;
    const value = window.unit === 'currency'
        ? window.available.toLocaleString('en-US', { maximumFractionDigits: 2 })
        : formatNumber(window.available);
    return `${value}${unit ? ` ${unit}` : ''}`;
}

export function formatQuotaUsage(
    window: QuotaUsageValues,
    { formatNumber = String }: FormatQuotaUsageOptions = {}
): string {
    if (!isCountable(window)) {
        const available = formatQuotaAvailable(window, formatNumber);
        if (available !== undefined) return available;
        // No figure to show, so show none — "0 / 0 (0%)" would read as unused.
        return window.unknown ? 'not reported' : 'no limit';
    }
    if (window.unit === 'percent') {
        return `${formatNumber(window.used)}% / ${formatNumber(window.limit)}%`;
    }

    return `${formatNumber(window.used)} / ${formatNumber(window.limit)} ${window.unit}`;
}
