// Re-export quota-related types from codegen
import type {
    UsageWindow,
    UsageCost,
    UsageAccount,
    UsageBreakdown,
    ProviderUsage,
} from '@/client';

/**
 * A quota window plus the semantics fields. Declared here because `task
 * codegen` has not yet regenerated the client schema; they ship on the wire
 * today. See .design/quota-semantics.md.
 */
export type QuotaWindow = UsageWindow & {
    kind?: 'limit' | 'resource';
    unknown?: boolean;
    unlimited?: boolean;
    available?: number;
    currency_code?: string;
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

// Type aliases for convenience and backward compatibility
export type ProviderQuota = ProviderUsage & {
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
    includePercent?: boolean;
    formatNumber?: (value: number) => string;
}

type QuotaUsageValues = CountableFields & Pick<QuotaWindow, 'used' | 'used_percent' | 'unit' | 'available' | 'currency_code'>;

export function formatQuotaPercent(window: QuotaUsageValues): string {
    return `${window.used_percent.toFixed(0)}%`;
}

export function formatQuotaAvailable(
    window: Pick<QuotaWindow, 'available' | 'currency_code' | 'unit'>,
    formatNumber: (value: number) => string = String
): string | undefined {
    if (window.available === undefined) return undefined;

    const unit = window.currency_code || window.unit;
    const value = window.unit === 'currency'
        ? window.available.toLocaleString('en-US', { maximumFractionDigits: 2 })
        : formatNumber(window.available);
    return `${value}${unit ? ` ${unit}` : ''}`;
}

export function formatQuotaUsage(
    window: QuotaUsageValues,
    { includePercent = false, formatNumber = String }: FormatQuotaUsageOptions = {}
): string {
    if (!isCountable(window)) {
        const available = formatQuotaAvailable(window, formatNumber);
        if (available !== undefined) return available;
        // No figure to show, so show none — "0 / 0 (0%)" would read as unused.
        return window.unknown ? 'not reported' : 'no limit';
    }
    if (window.unit === 'percent') {
        const usage = `${formatNumber(window.used)}% / ${formatNumber(window.limit)}%`;
        return includePercent ? `${usage} (${formatQuotaPercent(window)})` : usage;
    }

    const usage = `${formatNumber(window.used)} / ${formatNumber(window.limit)} ${window.unit}`;
    return includePercent ? `${usage} (${formatQuotaPercent(window)})` : usage;
}
