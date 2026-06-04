import React from 'react';
import { Chip } from '@mui/material';
import type { SxProps } from '@mui/material';


interface ProtocolBadgeProps {
  protocol: 'OpenAI' | 'Anthropic';
  size?: 'small' | 'medium';
  sx?: SxProps;
  onClick?: () => void;
}

const protocolConfig: Record<
  string,
  { label: string; color: string; bgcolor: string }
> = {
  OpenAI: {
    label: 'OpenAI',
    color: 'text.secondary',
    bgcolor: 'action.hover',
  },
  Anthropic: {
    label: 'Anthropic',
    color: 'text.secondary',
    bgcolor: 'action.hover',
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
      variant="outlined"
      onClick={onClick}
      sx={{
        height: size === 'small' ? 20 : 24,
        fontSize: '0.7rem',
        fontWeight: 500,
        color: config.color,
        bgcolor: config.bgcolor,
        borderColor: 'divider',
        cursor: onClick ? 'pointer' : 'default',
        '&:hover': onClick ? {
          opacity: 0.85,
        } : {},
        ...sx,
      }}
    />
  );
};

export default ProtocolBadge;
