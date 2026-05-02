// Re-export quota-related types from codegen
import type {
    UsageWindow,
    UsageCost,
    UsageAccount,
    UsageBreakdown,
    ProviderUsage,
} from '@/client';

// Type aliases for convenience and backward compatibility
// ProviderQuota is an alias for ProviderUsage from codegen
export type ProviderQuota = ProviderUsage;

// Re-export for consumers
export type { UsageWindow, UsageCost, UsageAccount, UsageBreakdown, ProviderUsage as ProviderUsage };

// Quota types for provider usage/limit information
// Note: Most types are now from codegen, see ../client/index.ts

// Extended quota with breakdowns flattened for UI consumption
export interface QuotaDisplayItem {
    key: string;           // Unique identifier (e.g., model name or "primary")
    label: string;         // Display label
    group?: string;        // Group type ("model", "type", or undefined for aggregate)
    windows: {
        primary?: UsageWindow;
        secondary?: UsageWindow;
        tertiary?: UsageWindow;
    };
}

// Helper to convert ProviderQuota to display items
export function quotaToDisplayItems(quota: ProviderQuota): QuotaDisplayItem[] {
    const items: QuotaDisplayItem[] = [];

    // Add breakdowns first (per-model or per-type)
    if (quota.breakdowns && quota.breakdowns.length > 0) {
        for (const bd of quota.breakdowns) {
            // Find daily and weekly windows for this breakdown
            const daily = bd.windows.find(w => w.type === 'daily');
            const weekly = bd.windows.find(w => w.type === 'weekly');

            items.push({
                key: bd.key,
                label: bd.label,
                group: bd.group,
                windows: {
                    primary: daily,
                    secondary: weekly,
                },
            });
        }
    }

    // Add aggregate item at the end
    items.push({
        key: 'aggregate',
        label: 'Total',
        windows: {
            primary: quota.primary,
            secondary: quota.secondary,
            tertiary: quota.tertiary,
        },
    });

    return items;
}
