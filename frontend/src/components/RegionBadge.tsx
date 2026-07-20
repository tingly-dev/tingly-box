import type { ChipProps, SxProps } from '@mui/material';
import { Chip } from '@mui/material';
import { LocationOn as LocationOnIcon } from '@/components/icons';
import { EMPTY_SX } from '@/constants/defaults';

export interface RegionBadgeProps {
  region: 'cn' | 'global' | 'self-hosted';
  size?: 'small' | 'medium';
  sx?: SxProps;
}

const getRegionColor = (region: 'cn' | 'global' | 'self-hosted'): { bg: string; text: string; icon: React.ReactNode } => {
  if (region === 'cn') {
    return {
      bg: 'error.50',
      text: 'error.main',
      icon: <LocationOnIcon sx={{ fontSize: 12 }} />,
    };
  }
  if (region === 'self-hosted') {
    return {
      bg: 'warning.50',
      text: 'warning.main',
      icon: <LocationOnIcon sx={{ fontSize: 12 }} />,
    };
  }
  return {
    bg: 'primary.50',
    text: 'primary.main',
    icon: <LocationOnIcon sx={{ fontSize: 12 }} />,
  };
};

const sizeStyles: Record<'small' | 'medium', { height: number; fontSize: string }> = {
  small: {
    height: 18,
    fontSize: '0.65rem',
  },
  medium: {
    height: 22,
    fontSize: '0.7rem',
  },
};

const RegionBadge: React.FC<RegionBadgeProps> = ({ region, size = 'small', sx = EMPTY_SX }) => {
  const colors = getRegionColor(region);
  const sizeStyle = sizeStyles[size];

  return (
    <Chip
      icon={colors.icon as React.ReactElement}
      label={region === 'cn' ? 'CN' : region === 'self-hosted' ? 'Self-hosted' : 'Global'}
      sx={{
        height: sizeStyle.height,
        fontSize: sizeStyle.fontSize,
        fontWeight: 600,
        bgcolor: colors.bg,
        color: colors.text,
        border: 'none',
        '& .MuiChip-icon': {
          color: colors.text,
          fontSize: 12,
        },
        ...sx,
      }}
    />
  );
};

export default RegionBadge;
