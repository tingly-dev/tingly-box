import { Box, Stack, IconButton, Typography, CircularProgress, Tooltip } from '@mui/material';
import RefreshIcon from '@mui/icons-material/Refresh';
import InfoIcon from '@mui/icons-material/Info';
import { QuotaBarItem } from './QuotaBarItem';
import type { ProviderQuota } from '@/types/quota';

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
  // Debug logging at component entry
  console.log('[QuotaInlineDisplay] Component rendered, quota:', quota);

  // Collect all available windows
  const windows: Array<{ key: string; window: any; label: string }> = [];

  console.log('[QuotaInlineDisplay] Checking quota.primary:', quota?.primary);

  if (quota?.primary) {
    console.log('[QuotaInlineDisplay] Found primary window:', quota.primary);
    windows.push({ key: 'primary', window: quota.primary, label: quota.primary.label });
  }
  if (quota?.secondary) {
    windows.push({ key: 'secondary', window: quota.secondary, label: quota.secondary.label });
  }
  if (quota?.tertiary) {
    windows.push({ key: 'tertiary', window: quota.tertiary, label: quota.tertiary.label });
  }

  const hasQuota = windows.length > 0;
  const hasHiddenItems = windows.length > maxInlineItems;
  const visibleWindows = windows.slice(0, maxInlineItems);
  const hiddenWindows = windows.slice(maxInlineItems);

  console.log('[QuotaInlineDisplay] windows collected:', windows);
  console.log('[QuotaInlineDisplay] hasQuota:', hasQuota);

  // No quota available - don't show anything
  if (!hasQuota) {
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
            {window.used === 0 && window.limit === 0 && window.unit === 'percent'
              ? `${window.used_percent.toFixed(0)}%`
              : `${window.used} / ${window.limit} ${window.unit} (${window.used_percent.toFixed(0)}%)`
            }
          </Typography>
          {window.resets_at && (
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
              Resets: {new Date(window.resets_at).toLocaleString()}
            </Typography>
          )}
        </Box>
      ))}
      {quota.cost && (
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
        px: 2,
        py: 1,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'flex-end',
        gap: 2,
      }}
    >
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
      </Stack>

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
      </Stack>
    </Box>
  );
}
