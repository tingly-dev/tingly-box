import { Box, Tooltip } from '@mui/material';
import { QuotaTooltipContent } from './QuotaTooltip';
import type { QuotaTooltipData, QuotaWindowDisplay } from './QuotaTooltip';
import { QUOTA_COLORS } from '../dashboard/chartStyles';
import type { ProviderQuota, TieredUsageWindow } from '../../types/quota';

interface QuotaBarProps {
  quota: ProviderQuota;
  window?: TieredUsageWindow;
  windowIndex?: 0 | 1 | 2;  // Legacy fallback only
}

export function QuotaBar({ quota, window: explicitWindow, windowIndex = 0 }: QuotaBarProps) {
  const window = explicitWindow ?? quota.windows?.[windowIndex] ?? null;
  if (!window) return null;

  // Get breakdown windows for tooltip
  const breakdownDisplays: QuotaWindowDisplay[] = [];
  if (quota.breakdowns && quota.breakdowns.length > 0) {
    for (const bd of quota.breakdowns) {
      const targetWindow = bd.windows.find(w => window.key && w.key === window.key)
        || bd.windows.find(w => w.type === window.type)
        || bd.windows[0];
      if (targetWindow) {
        breakdownDisplays.push({
          label: bd.label,
          window: targetWindow,
          group: bd.group,
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
