import { Box, Stack, Tooltip, Typography, tooltipClasses } from '@mui/material';
import type { UsageWindow } from '@/types/quota';
import { QUOTA_COLORS } from '../dashboard/chartStyles';

interface QuotaBarItemProps {
  window: UsageWindow;
  /**
   * Whether to show detailed info (used/limit, reset time)
   * If false, only shows label, bar, and percent
   * @default false
   */
  showDetails?: boolean;
}

/**
 * Compact inline display of a single quota window.
 * Shows: Label + Bar + Percent, with details in tooltip.
 */
export function QuotaBarItem({ window, showDetails = false }: QuotaBarItemProps) {
  const getColor = (percent: number) => {
    if (percent >= 80) return QUOTA_COLORS.error;
    if (percent >= 50) return QUOTA_COLORS.warning;
    return QUOTA_COLORS.success;
  };

  const barColor = getColor(window.used_percent);

  // Format usage display
  const formatUsageDisplay = () => {
    // For percentage-only quotas
    if (window.used === 0 && window.limit === 0 && window.unit === 'percent') {
      return `${window.used_percent.toFixed(0)}%`;
    }
    return `${window.used_percent.toFixed(0)}%`;
  };

  // Format detailed info for tooltip
  const formatDetailedInfo = () => {
    if (window.used === 0 && window.limit === 0 && window.unit === 'percent') {
      return `${window.used_percent.toFixed(0)}%`;
    }

    const formatNumber = (num: number) => {
      if (num >= 1000000) return `${(num / 1000000).toFixed(1)}M`;
      if (num >= 1000) return `${(num / 1000).toFixed(0)}K`;
      return num.toString();
    };

    return `${formatNumber(window.used)} / ${formatNumber(window.limit)} ${window.unit}`;
  };

  // Format reset time
  const formatResetTime = () => {
    if (!window.resets_at) return null;

    const resetDate = new Date(window.resets_at);
    const now = new Date();
    const diffMs = resetDate.getTime() - now.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);

    if (diffMs < 0) return 'Expired';
    if (diffMins < 60) return `in ${diffMins} min`;
    if (diffHours < 24) return `in ${diffHours}h`;
    if (diffDays < 7) return `in ${diffDays} days`;
    return resetDate.toLocaleDateString();
  };

  const resetTime = formatResetTime();
  const detailedInfo = formatDetailedInfo();

  const tooltipContent = (
    <Box
      sx={{
        backgroundColor: 'background.paper',
        border: '1px solid',
        borderColor: 'divider',
        borderRadius: 1,
        p: 1.5,
                        maxWidth: 250,
      }}
    >
      <Typography variant="caption" sx={{ fontWeight: 600, display: 'block', mb: 0.5 }}>
        {window.label}
      </Typography>
      <Typography variant="body2" sx={{ display: 'block', mb: 0.5 }}>
        {detailedInfo}
      </Typography>
      {resetTime && (
        <Typography variant="caption" color="text.secondary">
          Resets: {resetTime}
        </Typography>
      )}
      {window.description && (
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
          {window.description}
        </Typography>
      )}
    </Box>
  );

  return (
    <Tooltip
      title={tooltipContent}
      arrow
      disableInteractive
      componentsProps={{
        tooltip: {
          sx: {
            backgroundColor: 'transparent',
            boxShadow: 'none',
            padding: 0,
            [`& .${tooltipClasses.arrow}`]: {
              color: 'divider',
            },
          },
        },
      }}
    >
      <Stack
        direction="row"
        alignItems="center"
        spacing={1}
        sx={{
          flexShrink: 0,
        }}
      >
        {/* Label */}
        <Typography
          variant="body2"
          color="text.secondary"
          sx={{ minWidth: 45 }}
        >
          {window.label}:
        </Typography>

        {/* Bar */}
        <Box
          sx={{
            position: 'relative',
            width: 60,
            height: 8,
            cursor: 'pointer',
          }}
        >
          {/* Background */}
          <Box
            sx={{
              height: '100%',
              bgcolor: QUOTA_COLORS.background,
              borderRadius: 1,
              position: 'relative',
              overflow: 'hidden',
            }}
          >
            {/* Fill bar */}
            <Box
              sx={{
                height: '100%',
                width: `${Math.min(window.used_percent, 100)}%`,
                bgcolor: barColor,
                borderRadius: 1,
                transition: 'width 0.3s ease',
              }}
            />
          </Box>
        </Box>

        {/* Percent */}
        <Typography
          variant="body2"
          sx={{
            color: barColor,
            minWidth: 35,
          }}
        >
          {formatUsageDisplay()}
        </Typography>

        {/* Optional details inline */}
        {showDetails && (
          <>
            <Typography variant="caption" color="text.secondary">
              {detailedInfo}
            </Typography>
            {resetTime && (
              <Typography variant="caption" color="text.secondary">
                ({resetTime})
              </Typography>
            )}
          </>
        )}
      </Stack>
    </Tooltip>
  );
}
