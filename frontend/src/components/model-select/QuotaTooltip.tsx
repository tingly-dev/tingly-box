import { Box, Typography } from '@mui/material';
import { tooltipStyle, tooltipTextStyles, formatNumber } from '../dashboard/chartStyles';
import type { UsageWindow } from '../../types/quota';

export interface QuotaTooltipData {
  label: string;
  used: number;
  limit: number;
  percent: number;
  unit: string;
  resetsAt?: string;
  color?: string;
}

export interface QuotaWindowDisplay {
  label: string;
  window: UsageWindow;
  color?: string;
}

export interface QuotaTooltipProps {
  title: string;
  primary: QuotaTooltipData;
  secondary?: QuotaTooltipData;
  cost?: {
    used: number;
    limit: number;
    currency?: string;
  };
  breakdowns?: QuotaWindowDisplay[];  // Breakdown items (per-model/type)
}

export function QuotaTooltipContent({ title, primary, secondary, cost, breakdowns }: QuotaTooltipProps) {
  // Helper function to format usage display
  const formatUsageDisplay = (data: QuotaTooltipData) => {
    // For percentage-only quotas (used=0, limit=0, unit=percent)
    if (data.used === 0 && data.limit === 0 && data.unit === 'percent') {
      return `${data.percent.toFixed(0)}%`;
    }
    return `${formatNumber(data.used)} / ${formatNumber(data.limit)} ${data.unit}`;
  };

  return (
    <Box sx={tooltipStyle}>
      <Typography sx={tooltipTextStyles.title}>
        {title}
      </Typography>

      {/* Primary usage */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 1,
          mb: 0.5,
        }}
      >
        <Box
          sx={{
            width: 12,
            height: 12,
            borderRadius: 2,
            backgroundColor: primary.color || '#10b981',
          }}
        />
        <Typography sx={tooltipTextStyles.body}>
          {formatUsageDisplay(primary)}
        </Typography>
      </Box>

      {primary.resetsAt && (
        <Typography
          sx={{
            ...tooltipTextStyles.caption,
            display: 'block',
            ml: 3.25,
          }}
        >
          Resets: {new Date(primary.resetsAt).toLocaleString()}
        </Typography>
      )}

      {secondary && (
        <Box
          sx={{
            mt: 1.5,
            pt: 1,
            borderTop: tooltipTextStyles.divider,
          }}
        >
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 1,
            }}
          >
            <Box
              sx={{
                width: 12,
                height: 12,
                borderRadius: 2,
                backgroundColor: secondary.color || '#94a3b8',
              }}
            />
            <Typography sx={tooltipTextStyles.body}>
              {secondary.label}: {formatUsageDisplay(secondary)}
            </Typography>
          </Box>
        </Box>
      )}

      {/* Breakdowns (per-model or per-type) */}
      {breakdowns && breakdowns.length > 0 && (
        <Box
          sx={{
            mt: 1.5,
            pt: 1,
            borderTop: tooltipTextStyles.divider,
          }}
        >
          <Typography sx={{ ...tooltipTextStyles.caption, fontWeight: 500, mb: 1 }}>
            By {breakdowns[0]?.window?.type === 'daily' ? 'Model' : 'Type'}:
          </Typography>
          {breakdowns.map((bd, idx) => (
            <Box
              key={bd.label}
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 1,
                mb: idx < breakdowns.length - 1 ? 0.5 : 0,
              }}
            >
              <Box
                sx={{
                  width: 10,
                  height: 10,
                  borderRadius: 1.5,
                  backgroundColor: bd.color || '#64748b',
                }}
              />
              <Typography sx={{ ...tooltipTextStyles.caption, fontSize: '11px' }}>
                {bd.label}: {bd.window.used === 0 && bd.window.limit === 0 && bd.window.unit === 'percent'
                  ? `${bd.window.used_percent.toFixed(0)}%`
                  : `${formatNumber(bd.window.used)} / ${formatNumber(bd.window.limit)} (${bd.window.used_percent.toFixed(0)}%)`
                }
              </Typography>
            </Box>
          ))}
        </Box>
      )}

      {cost && (
        <Box
          sx={{
            mt: 1,
            pt: 1,
            borderTop: tooltipTextStyles.divider,
          }}
        >
          <Typography
            sx={{
              ...tooltipTextStyles.body,
              fontWeight: 500,
            }}
          >
            💰 Cost: {cost.currency || '$'}{cost.used.toFixed(2)} / {cost.currency || '$'}{cost.limit.toFixed(2)}
          </Typography>
        </Box>
      )}
    </Box>
  );
}
