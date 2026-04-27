import React from 'react';
import { Chip } from '@mui/material';
import type { SxProps } from '@mui/material';
import type { ChipProps } from '@mui/material';

interface ProtocolBadgeProps {
  protocol: 'OpenAI' | 'Anthropic';
  size?: 'small' | 'medium';
  sx?: SxProps;
  onClick?: () => void;
}

const protocolConfig: Record<
  string,
  { label: string; color: ChipProps['color']; bgcolor: string }
> = {
  OpenAI: {
    label: 'OpenAI',
    color: 'primary' as const,
    bgcolor: 'primary.50',
  },
  Anthropic: {
    label: 'Anthropic',
    color: 'info' as const,
    bgcolor: 'info.50',
  },
};

const ProtocolBadge: React.FC<ProtocolBadgeProps> = ({
  protocol,
  size = 'small',
  sx = {},
  onClick,
}) => {
  const config = protocolConfig[protocol];

  return (
    <Chip
      label={config.label}
      size={size}
      color={config.color}
      onClick={onClick}
      sx={{
        bgcolor: config.bgcolor,
        fontWeight: 500,
        cursor: onClick ? 'pointer' : 'default',
        '&:hover': onClick ? {
          bgcolor: `${config.bgcolor}.dark`,
          opacity: 0.9,
        } : {},
        ...sx,
      }}
    />
  );
};

export default ProtocolBadge;
