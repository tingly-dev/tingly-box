import React from 'react';
import { Box, Stack, Typography } from '@mui/material';
import type { ProviderQuota, UsageWindow } from '@/types/quota';
import { quotaToWindows } from '@/types/quota';
import { QuotaBarItem } from './QuotaBarItem';

interface QuotaBarRowProps {
  quota: ProviderQuota | undefined;
}

interface ResourceItem {
  key: string;
  window: UsageWindow;
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
          used_percent: 100,
          unit: 'percent' as const,
        } as UsageWindow,
        countLabel: `${total}`,
        tooltipContent,
      };
    });
  }, [quota]);

  const hasAny = windows.length > 0 || resourceItems.length > 0;

  return { windows, resourceItems, hasAny };
}

/**
 * Horizontal row of quota bar items — percentage windows + resource items.
 * Shared between credential detail rows and model-select panels.
 */
export function QuotaBarRow({ quota }: QuotaBarRowProps) {
  const { windows, resourceItems, hasAny } = useQuotaBars(quota);

  if (!hasAny) return null;

  return (
    <Stack
      direction="row"
      spacing={2}
      alignItems="center"
      sx={{
        overflowX: 'auto',
        '&::-webkit-scrollbar': { display: 'none' },
        msOverflowStyle: 'none',
        scrollbarWidth: 'none',
      }}
    >
      {windows.map(({ key, window }) => (
        <QuotaBarItem key={key} window={window} />
      ))}
      {resourceItems.map(item => (
        <QuotaBarItem
          key={item.key}
          window={item.window}
          percentLabel={item.countLabel}
          barColor="#22c55e"
          tooltipContent={item.tooltipContent}
        />
      ))}
    </Stack>
  );
}
