import { Box, Typography } from '@mui/material';
import { tooltipStyle, tooltipTextStyles, formatNumber } from '../dashboard/chartStyles';
import { formatQuotaRemaining, formatQuotaUsage, isCountable } from '../../types/quota';
import type { UsageWindow } from '../../types/quota';

export interface QuotaTooltipData {
  label: string;
  used: number;
  limit: number;
  percent: number;
  available?: number;
  unknown?: boolean;
  unlimited?: boolean;
  unit: string;
  resetsAt?: string;
  color?: string;
}

export interface QuotaWindowDisplay {
  label: string;
  window: UsageWindow;
  group?: string;
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
  const asWindow = (data: QuotaTooltipData) => ({
    used: data.used,
    limit: data.limit,
    used_percent: data.percent,
    available: data.available,
    unknown: data.unknown,
    unlimited: data.unlimited,
    unit: data.unit,
  });
  const formatRemainingDisplay = (data: QuotaTooltipData) => formatQuotaRemaining(
    asWindow(data), formatNumber
  );

  return (
    <Box sx={tooltipStyle}>
      <Typography sx={tooltipTextStyles.title}>
        {title}
      </Typography>

      {/* Primary remaining quota */}
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
          {formatRemainingDisplay(primary)}{isCountable(asWindow(primary)) || primary.available != null ? ' remaining' : ''}
        </Typography>
      </Box>

      {isCountable(asWindow(primary)) && <Typography sx={{ ...tooltipTextStyles.caption, display: 'block', ml: 3.25 }}>
        Used: {formatQuotaUsage(asWindow(primary), { formatNumber })}
      </Typography>}

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
              {secondary.label}: {formatRemainingDisplay(secondary)}{isCountable(asWindow(secondary)) || secondary.available != null ? ' remaining' : ''}
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
            By {breakdowns[0]?.group || 'Item'}:
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
                {bd.label}: {formatQuotaRemaining(bd.window, formatNumber)}{isCountable(bd.window) || bd.window.available != null ? ' remaining' : ''}
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
            💰 Cost: {cost.limit > 0 ? `${cost.currency || '$'}${Math.max(0, cost.limit - cost.used).toFixed(2)} / ${cost.currency || '$'}${cost.limit.toFixed(2)} remaining` : `${cost.currency || '$'}${cost.used.toFixed(2)} used`}
          </Typography>
        </Box>
      )}
    </Box>
  );
}
