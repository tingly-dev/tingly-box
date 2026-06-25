// Re-export quota-related types from codegen
import type {
    UsageWindow,
    UsageCost,
    UsageAccount,
    UsageBreakdown,
    ProviderUsage,
} from '@/client';

export type TieredUsageWindow = UsageWindow & {
    key?: string;
    tier?: number;
};

// Type aliases for convenience and backward compatibility
export type ProviderQuota = ProviderUsage & {
    windows?: TieredUsageWindow[];
};

// Re-export for consumers
export type { UsageWindow, UsageCost, UsageAccount, UsageBreakdown, ProviderUsage as ProviderUsage };

// Quota types for provider usage/limit information
// Note: Most types are now from codegen, see ../client/index.ts

export interface QuotaWindowDisplayItem {
    key: string;
    label: string;
    window: TieredUsageWindow;
}

function windowPercent(window: TieredUsageWindow): number {
    if (typeof window.used_percent === 'number') {
        return window.used_percent;
    }
    if (window.limit > 0) {
        return Math.min((window.used / window.limit) * 100, 100);
    }
    return 0;
}

function withWindowDefaults(
    window: TieredUsageWindow,
    key: string,
    tier: number
): QuotaWindowDisplayItem {
    return {
        key: window.key || key,
        label: window.label || key,
        window: {
            ...window,
            key: window.key || key,
            tier: window.tier ?? tier,
            used_percent: windowPercent(window),
        },
    };
}

export function quotaToWindows(quota?: ProviderQuota): QuotaWindowDisplayItem[] {
    if (!quota || !quota.windows?.length) return [];

    return quota.windows
        .map((window, index) => withWindowDefaults(window, `window-${index}`, 100 + index))
        .sort((a, b) => (a.window.tier ?? 999) - (b.window.tier ?? 999));
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
