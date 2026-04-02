import { Box, Tooltip } from '@mui/material';
import { QuotaTooltipContent, QuotaTooltipData, QuotaWindowDisplay } from './QuotaTooltip';
import { QUOTA_COLORS, formatNumber } from '../dashboard/chartStyles';
import type { ProviderQuota } from '../../types/quota';

interface UsageWindow {
  type: string;
  used: number;
  limit: number;
  used_percent: number;
  resets_at?: string;
  unit: string;
  label: string;
  description?: string;
}

interface UsageCost {
  used: number;
  limit: number;
  currency_code: string;
  label?: string;
}

interface QuotaBarProps {
  quota: ProviderQuota;
  windowIndex?: 0 | 1 | 2;  // 0=primary, 1=secondary, 2=tertiary
}

export function QuotaBar({ quota, windowIndex = 0 }: QuotaBarProps) {
  // Get the window based on index
  const getWindow = (): UsageWindow | null => {
    if (windowIndex === 0) return quota.primary || null;
    if (windowIndex === 1) return quota.secondary || null;
    if (windowIndex === 2) return quota.tertiary || null;
    return quota.primary || null;
  };

  const window = getWindow();
  if (!window) return null;

  // Get breakdown windows for tooltip
  const breakdownDisplays: QuotaWindowDisplay[] = [];
  if (quota.breakdowns && quota.breakdowns.length > 0) {
    for (const bd of quota.breakdowns) {
      // Use the window matching current window type
      const targetWindow = bd.windows.find(w => w.type === window.type) || bd.windows[0];
      if (targetWindow) {
        breakdownDisplays.push({
          label: bd.label,
          window: targetWindow,
          color: QUOTA_COLORS.secondary,
        });
      }
    }
  }

  // Get color based on usage
  const getColor = (percent: number) => {
    if (percent >= 80) return QUOTA_COLORS.error;
    if (percent >= 50) return QUOTA_COLORS.warning;
    return QUOTA_COLORS.success;
  };

  const barColor = getColor(window.used_percent);

  // Build primary tooltip data
  const primaryData: QuotaTooltipData = {
    label: window.label,
    used: window.used,
    limit: window.limit,
    percent: window.used_percent,
    unit: window.unit,
    resetsAt: window.resets_at,
    color: barColor,
  };

  const tooltipContent = (
    <QuotaTooltipContent
      title={window.label}
      primary={primaryData}
      breakdowns={breakdownDisplays}
    />
  );

  return (
    <Tooltip
      title={tooltipContent}
      arrow={false}
      placement="top"
      componentsProps={{
        tooltip: {
          sx: {
            backgroundColor: 'transparent',
            boxShadow: 'none',
            padding: 0,
            border: 'none',
          },
        },
      }}
    >
      <Box
        sx={{
          position: 'relative',
          width: '100%',
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

        {/* Current position indicator */}
        <Box
          sx={{
            position: 'absolute',
            left: `${Math.min(window.used_percent, 100)}%`,
            top: '50%',
            transform: 'translate(-50%, -50%)',
            width: 0,
            height: 0,
            borderLeft: '6px solid transparent',
            borderRight: '6px solid transparent',
            borderTop: `10px solid ${barColor}`,
            transition: 'left 0.3s ease',
          }}
        />
      </Box>
    </Tooltip>
  );
}

// Legacy props interface for backward compatibility
export interface QuotaBarLegacyProps {
  used: number;
  limit: number;
  percent: number;
  unit: string;
  label: string;
  resetsAt?: string;
  secondary?: UsageWindow;
  cost?: UsageCost;
}

export function QuotaBarLegacy({
  used,
  limit,
  percent,
  unit,
  label,
  resetsAt,
  secondary,
  cost,
}: QuotaBarLegacyProps) {
  const getColor = () => {
    if (percent >= 80) return QUOTA_COLORS.error;
    if (percent >= 50) return QUOTA_COLORS.warning;
    return QUOTA_COLORS.success;
  };

  const barColor = getColor();

  const primaryData: QuotaTooltipData = {
    label,
    used,
    limit,
    percent,
    unit,
    resetsAt: resetsAt,
    color: barColor,
  };

  const secondaryData = secondary ? {
    label: secondary.label,
    used: secondary.used,
    limit: secondary.limit,
    percent: secondary.used_percent,
    unit: secondary.unit,
    color: QUOTA_COLORS.secondary,
  } : undefined;

  const costData = cost ? {
    used: cost.used,
    limit: cost.limit,
    currency: cost.currency_code,
  } : undefined;

  const tooltipContent = (
    <QuotaTooltipContent
      title={label}
      primary={primaryData}
      secondary={secondaryData}
      cost={costData}
    />
  );

  return (
    <Tooltip
      title={tooltipContent}
      arrow={false}
      placement="top"
      componentsProps={{
        tooltip: {
          sx: {
            backgroundColor: 'transparent',
            boxShadow: 'none',
            padding: 0,
            border: 'none',
          },
        },
      }}
    >
      <Box
        sx={{
          position: 'relative',
          width: '100%',
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
              width: `${Math.min(percent, 100)}%`,
              bgcolor: barColor,
              borderRadius: 1,
              transition: 'width 0.3s ease',
            }}
          />
        </Box>

        {/* Current position indicator */}
        <Box
          sx={{
            position: 'absolute',
            left: `${Math.min(percent, 100)}%`,
            top: '50%',
            transform: 'translate(-50%, -50%)',
            width: 0,
            height: 0,
            borderLeft: '6px solid transparent',
            borderRight: '6px solid transparent',
            borderTop: `10px solid ${barColor}`,
            transition: 'left 0.3s ease',
          }}
        />
      </Box>
    </Tooltip>
  );
}
