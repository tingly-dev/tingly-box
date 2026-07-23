import React, { useState } from 'react';
import { Box, Button, Stack, IconButton, Typography, CircularProgress, Tooltip } from '@mui/material';
import { Code as CodeIcon } from '@/components/icons';
import { Refresh as RefreshIcon } from '@/components/icons';
import { Info as InfoIcon } from '@/components/icons';
import { QuotaBarItem } from './QuotaBarItem';
import { QuotaBarRow, useQuotaBars } from './QuotaBarRow';
import { QuotaRawResponseDialog } from './QuotaRawResponseDialog';
import type { ProviderQuota } from '@/types/quota';
import { formatQuotaUsage } from '@/types/quota';

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

function formatFetchedAgo(fetchedAt?: string): string {
  if (!fetchedAt) return 'Refresh';

  const diffMs = Math.max(0, Date.now() - new Date(fetchedAt).getTime());
  const mins = Math.floor(diffMs / 60000);
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins}m ago`;

  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  return `${Math.floor(hrs / 24)}d ago`;
}

/**
 * Horizontal inline display of quota information.
 * Shows quota bars with refresh/info actions.
 * Hidden items accessible via info icon tooltip.
 */
export function QuotaInlineDisplay({
  quota,
  isRefreshing,
  onRefresh,
  maxInlineItems = 3,
}: QuotaInlineDisplayProps) {
  const [rawResponseOpen, setRawResponseOpen] = useState(false);
  const { windows, resourceItems, hasAny } = useQuotaBars(quota);
  const hasRawResponse = quota?.raw_response !== undefined && quota.raw_response !== null;

  const hasHiddenItems = windows.length > maxInlineItems;
  const visibleWindows = windows.slice(0, maxInlineItems);
  const hiddenWindows = windows.slice(maxInlineItems);
  const fetchedAgo = formatFetchedAgo(quota?.fetched_at);

  // Show nothing if there's no data at all
  if (!hasAny && !hasRawResponse) {
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
    <>
      <Box
        sx={{
          pl: 8,
          pr: 2,
          py: 1,
          display: 'flex',
          alignItems: 'center',
          gap: 1.5,
        }}
      >
        {/* Freshness leads the row so users know when these values were captured. */}
        <Tooltip
          title={quota?.fetched_at
            ? `Refresh quota · Fetched at ${new Date(quota.fetched_at).toLocaleString()}`
            : 'Refresh quota'}
          arrow
        >
          <span>
            <Button
              aria-label="Refresh quota"
              size="small"
              variant="text"
              startIcon={isRefreshing ? <CircularProgress size={14} /> : <RefreshIcon sx={{ fontSize: 16 }} />}
              onClick={onRefresh}
              disabled={isRefreshing}
              sx={{
                minWidth: 0,
                px: 0.75,
                color: 'text.secondary',
                fontSize: '0.7rem',
                fontWeight: 400,
                textTransform: 'none',
                whiteSpace: 'nowrap',
                '& .MuiButton-startIcon': { mr: 0.5 },
                '&:hover': {
                  bgcolor: 'action.hover',
                  color: 'text.primary',
                },
              }}
            >
              {isRefreshing ? 'Refreshing' : fetchedAgo}
            </Button>
          </span>
        </Tooltip>

        {hasRawResponse && (
          <Tooltip title="View raw quota response" arrow>
            <Button
              aria-label="View raw quota response"
              size="small"
              variant="text"
              startIcon={<CodeIcon sx={{ fontSize: 16 }} />}
              onClick={() => setRawResponseOpen(true)}
              sx={{
                flexShrink: 0,
                px: 0.75,
                color: 'text.secondary',
                fontSize: '0.7rem',
                fontWeight: 400,
                textTransform: 'none',
                whiteSpace: 'nowrap',
                '& .MuiButton-startIcon': { mr: 0.5 },
                '&:hover': {
                  bgcolor: 'action.hover',
                  color: 'text.primary',
                },
              }}
            >
              Details
            </Button>
          </Tooltip>
        )}

        {/* Quota metrics remain the visual anchor. */}
        <Stack
          direction="row"
          spacing={2}
          alignItems="center"
          sx={{
            flex: 1,
            minWidth: 0,
            overflowX: 'auto',
            '&::-webkit-scrollbar': { display: 'none' },
            msOverflowStyle: 'none',
            scrollbarWidth: 'none',
          }}
        >
          {visibleWindows.map(({ key, window }) => (
            <QuotaBarItem key={key} window={window} />
          ))}
          {resourceItems.map(item => (
            <QuotaBarItem key={item.key} window={item.window} percentLabel={item.countLabel} barColor="#22c55e" tooltipContent={item.tooltipContent} />
          ))}
          {!hasAny && hasRawResponse && (
            <Typography variant="caption" color="text.disabled" sx={{ whiteSpace: 'nowrap' }}>
              No quota limits reported
            </Typography>
          )}
        </Stack>

        {hasHiddenItems && (
          <Tooltip title={hiddenItemsTooltip} arrow disableInteractive>
            <IconButton
              aria-label="View additional quota information"
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
      </Box>

      {hasRawResponse && (
        <QuotaRawResponseDialog
          open={rawResponseOpen}
          onClose={() => setRawResponseOpen(false)}
          providerName={quota?.provider_name}
          response={quota?.raw_response}
        />
      )}
    </>
  );
}
