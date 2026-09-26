import React from 'react';
import { Box, Typography } from '@mui/material';
import type { ProviderQuota, QuotaWindow } from '@/types/quota';
import { quotaToWindows } from '@/types/quota';

interface ResourceItem {
  key: string;
  window: QuotaWindow;
  countLabel: string;
  tooltipContent: React.ReactNode;
}

/**
 * Hook: computes windows + resource items from a ProviderQuota in one pass.
 * Returns the windows list, resource items, and whether there's anything to show.
 */
export function useQuotaBars(quota: ProviderQuota | undefined): {
  windows: ReturnType<typeof quotaToWindows>;
  resourceItems: ResourceItem[];
  hasAny: boolean;
} {
  const windows = React.useMemo(() => quotaToWindows(quota), [quota]);

  const resourceItems: ResourceItem[] = React.useMemo(() => {
    if (!quota) return [];
    // A tingly-box upstream already shows one bar per model (the backend
    // lifts each model's binding window into `windows`); its per-model
    // breakdowns exist for routing, and a count bar here would repeat them.
    if (quota.provider_type === 'tingly_box') return [];
    const breakdowns = quota.breakdowns;
    if (!breakdowns?.length) return [];

    const groups = new Map<string, typeof breakdowns>();
    for (const bd of breakdowns) {
      const list = groups.get(bd.group) ?? [];
      list.push(bd);
      groups.set(bd.group, list);
    }

    return Array.from(groups.entries()).map(([group, items]) => {
      const total = items.length;
      const label = group
        .split('_')
        .map(w => w.charAt(0).toUpperCase() + w.slice(1))
        .join(' ');

      const tooltipContent = (
        <Box sx={{ backgroundColor: 'background.paper', border: '1px solid', borderColor: 'divider', borderRadius: 1, p: 1.5, maxWidth: 250 }}>
          <Typography variant="caption" sx={{ fontWeight: 600, display: 'block', mb: 1 }}>
            {label} ({total})
          </Typography>
          {items.map((bd: any) => {
            const win = bd.windows?.[0];
            return win ? (
              <Typography key={bd.key} variant="caption" sx={{ color: 'text.secondary', display: 'block', mb: 0.3, lineHeight: 1.4 }}>
                {(bd.label || bd.key)}{win.description ? `: ${win.description}` : ''}
              </Typography>
            ) : null;
          })}
        </Box>
      );

      return {
        key: group,
        window: {
          label,
          used: 0,
          limit: total,
          used_percent: 0,
          unit: 'percent' as const,
        } as QuotaWindow,
        countLabel: `${total}`,
        tooltipContent,
      };
    });
  }, [quota]);

  const hasAny = windows.length > 0 || resourceItems.length > 0;

  return { windows, resourceItems, hasAny };
}
