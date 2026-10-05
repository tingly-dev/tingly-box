import React from 'react';
import { Box, Typography } from '@mui/material';
import type { ProviderQuota, QuotaWindow } from '@/types/quota';
import { quotaToWindows } from '@/types/quota';

export interface ResourceEntry {
  key: string;
  label: string;
  window: QuotaWindow;
}

export interface ResourceItem {
  key: string;
  window: QuotaWindow;
  /** "3/4" for vouchers (available / total), otherwise the number of entries. */
  countLabel: string;
  /** Share still available, for a ring; only vouchers have one. */
  remaining?: number;
  /** The individual entries of the group, for the detailed hover. */
  entries: ResourceEntry[];
  tooltipContent: React.ReactNode;
}

/** A voucher is a one-shot credit (Codex reset credits): a count, not a share. */
function isVoucher(window: QuotaWindow | undefined): boolean {
  return !!window && window.unit === 'credits' && window.limit === 1;
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
      const entries: ResourceEntry[] = items.flatMap((bd: any) =>
        bd.windows?.[0] ? [{ key: bd.key, label: bd.label || bd.key, window: bd.windows[0] as QuotaWindow }] : []);
      const voucher = entries.length > 0 && entries.every(e => isVoucher(e.window));
      const available = voucher ? entries.filter(e => e.window.used < e.window.limit).length : 0;
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
        countLabel: voucher ? `${available}/${total}` : `${total}`,
        remaining: voucher ? available / total * 100 : undefined,
        entries,
        tooltipContent,
      };
    });
  }, [quota]);

  const hasAny = windows.length > 0 || resourceItems.length > 0;

  return { windows, resourceItems, hasAny };
}
