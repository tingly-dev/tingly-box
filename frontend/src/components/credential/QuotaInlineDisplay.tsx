import React from 'react';
import { Box, Stack, IconButton, Typography, CircularProgress, Tooltip } from '@mui/material';
import { AccessTime as AccessTimeIcon } from '@/components/icons';
import { Refresh as RefreshIcon } from '@/components/icons';
import { Info as InfoIcon } from '@/components/icons';
import { QuotaBarItem } from './QuotaBarItem';
import type { ProviderQuota, UsageWindow } from '@/types/quota';
import { formatQuotaUsage, quotaToWindows } from '@/types/quota';

interface QuotaInlineDisplayProps {
  quota: ProviderQuota | undefined;
  isRefreshing: boolean;
  onRefresh: () => void;
  /**
   * Maximum number of quota items to display inline
   * Additional items are available via info tooltip
   * @default 3
   */
  maxInlineItems?: number;
}

/**
 * Horizontal inline display of quota information.
 * Shows quota items side by side with refresh button.
 * Hidden items accessible via info icon tooltip.
 */
export function QuotaInlineDisplay({
  quota,
  isRefreshing,
  onRefresh,
  maxInlineItems = 3,
}: QuotaInlineDisplayProps) {
  const windows = quotaToWindows(quota);

  const hasQuota = windows.length > 0;
  const hasHiddenItems = windows.length > maxInlineItems;
  const visibleWindows = windows.slice(0, maxInlineItems);
  const hiddenWindows = windows.slice(maxInlineItems);

  // Compute reset credits synthetic window from breakdowns
  const resetCreditsWindow: { key: string; window: UsageWindow; countLabel: string; tooltipContent: React.ReactNode } | null = (() => {
    if (!quota) return null;
    const credits = (quota.breakdowns ?? []).filter(bd => bd.group === 'reset_credit');
    if (credits.length === 0) return null;
    const available = credits.filter(bd => bd.windows[0]?.label === 'available').length;
    const total = credits.length;
    const used = total - available;

    // Build per-credit detail tooltip
    const STATUS_COLORS: Record<string, string> = {
      available: '#22c55e',
      used: '#94a3b8',
      expired: '#ef4444',
    };
    const tooltipContent = (
      <Box sx={{ backgroundColor: 'background.paper', border: '1px solid', borderColor: 'divider', borderRadius: 1, p: 1.5, maxWidth: 250 }}>
        <Typography variant="caption" sx={{ fontWeight: 600, display: 'block', mb: 1 }}>
          Reset Credits ({available} / {total} available)
        </Typography>
        {credits.map((bd) => {
          const win = bd.windows[0];
          if (!win) return null;
          const status = win.label || 'unknown';
          const color = STATUS_COLORS[status] || STATUS_COLORS.used;
          const expiresAt = win.resets_at ? new Date(win.resets_at) : null;
          return (
            <Stack key={bd.key} direction="row" spacing={1} alignItems="center" sx={{ mb: 0.5 }}>
              <Box sx={{ width: 6, height: 6, borderRadius: '50%', bgcolor: color, flexShrink: 0 }} />
              <Typography variant="caption" sx={{ color: 'text.secondary', lineHeight: 1.3 }}>
                {status === 'available' ? 'Available' : status.charAt(0).toUpperCase() + status.slice(1)}
                {expiresAt && ` · Expires ${expiresAt.toLocaleDateString()}`}
              </Typography>
            </Stack>
          );
        })}
      </Box>
    );

    return {
      window: {
        label: 'Reset Credits',
        used: used,
        limit: total,
        used_percent: 100,
        unit: 'percent' as const,
        description: `${available} available, ${total} total`,
      } as UsageWindow,
      key: 'reset_credits',
      countLabel: `${available}/${total}`,
      tooltipContent,
    };
  })();

  // Show nothing if there's no data at all
  if (!hasQuota && !resetCreditsWindow) {
    return null;
  }

  // Build hidden items tooltip content
  const hiddenItemsTooltip = hasHiddenItems ? (
    <Box
      sx={{
        backgroundColor: 'background.paper',
        border: '1px solid',
        borderColor: 'divider',
        borderRadius: 1,
        p: 1.5,
        maxWidth: 300,
      }}
    >
      <Typography variant="caption" sx={{ fontWeight: 600, display: 'block', mb: 1 }}>
        Additional Quota Information
      </Typography>
      {hiddenWindows.map(({ key, window, label }) => (
        <Box key={key} sx={{ mb: 1 }}>
          <Typography variant="caption" sx={{ fontWeight: 500, display: 'block' }}>
            {label}
          </Typography>
          <Typography variant="caption" color="text.secondary">
            {formatQuotaUsage(window, { includePercent: true })}
          </Typography>
          {window.resets_at && (
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
              Resets: {new Date(window.resets_at).toLocaleString()}
            </Typography>
          )}
        </Box>
      ))}
      {quota?.cost && (
        <Box sx={{ mt: 1.5, pt: 1, borderTop: '1px solid', borderColor: 'divider' }}>
          <Typography variant="caption" sx={{ fontWeight: 500, display: 'block' }}>
            Cost
          </Typography>
          <Typography variant="caption" color="text.secondary">
            {quota.cost.currency_code || '$'}{quota.cost.used.toFixed(2)} / {quota.cost.currency_code || '$'}{quota.cost.limit.toFixed(2)}
          </Typography>
        </Box>
      )}
    </Box>
  ) : null;

  return (
    <Box
      sx={{
        pl: 8,
        pr: 2,
        py: 1,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'flex-start',
        gap: 2,
      }}
    >
      {/* Actions */}
      <Stack direction="row" spacing={1} alignItems="center">
        {/* Info icon for hidden items */}
        {hasHiddenItems && (
          <Tooltip title={hiddenItemsTooltip} arrow disableInteractive>
            <IconButton
              size="small"
              sx={{
                p: 0.5,
                color: 'text.secondary',
                '&:hover': {
                  bgcolor: 'action.hover',
                },
              }}
            >
              <InfoIcon fontSize="small" />
            </IconButton>
          </Tooltip>
        )}

        {/* Refresh button */}
        <Tooltip title="Refresh quota" arrow>
          <IconButton
            size="small"
            onClick={onRefresh}
            disabled={isRefreshing}
            sx={{
              p: 0.5,
              color: 'text.secondary',
              '&:hover': {
                bgcolor: 'action.hover',
              },
              '&:disabled': {
                color: 'text.disabled',
              },
            }}
          >
            {isRefreshing ? (
              <CircularProgress size={16} />
            ) : (
              <RefreshIcon fontSize="small" />
            )}
          </IconButton>
        </Tooltip>

        {/* Fetched time */}
        {quota?.fetched_at && !isRefreshing && (
          <Tooltip title={`Fetched at ${new Date(quota.fetched_at).toLocaleString()}`} arrow>
            <Stack direction="row" alignItems="center" spacing={0.5} sx={{ cursor: 'default' }}>
              <AccessTimeIcon sx={{ fontSize: 12, color: 'text.disabled' }} />
              <Typography variant="caption" color="text.disabled" sx={{ whiteSpace: 'nowrap' }}>
                {(() => {
                  const diffMs = Date.now() - new Date(quota.fetched_at!).getTime();
                  const mins = Math.floor(diffMs / 60000);
                  if (mins < 1) return 'just now';
                  if (mins < 60) return `${mins}m ago`;
                  const hrs = Math.floor(mins / 60);
                  if (hrs < 24) return `${hrs}h ago`;
                  return `${Math.floor(hrs / 24)}d ago`;
                })()}
              </Typography>
            </Stack>
          </Tooltip>
        )}
      </Stack>

      {/* Quota items */}
      <Stack
        direction="row"
        spacing={2}
        sx={{
          overflowX: 'auto',
          // Hide scrollbar but keep functionality
          '&::-webkit-scrollbar': {
            display: 'none',
          },
          msOverflowStyle: 'none',
          scrollbarWidth: 'none',
        }}
      >
        {visibleWindows.map(({ key, window }) => (
          <QuotaBarItem key={key} window={window} />
        ))}
        {resetCreditsWindow && (
          <QuotaBarItem key={resetCreditsWindow.key} window={resetCreditsWindow.window} percentLabel={resetCreditsWindow.countLabel} barColor="#22c55e" tooltipContent={resetCreditsWindow.tooltipContent} />
        )}
      </Stack>
    </Box>
  );
}
