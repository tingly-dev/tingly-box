import { IconButton, Tooltip, Typography } from '@mui/material';
import type { SxProps } from '@mui/system';
import type { Theme } from '@mui/material/styles';
import React, {type ReactNode } from 'react';

export interface ZenButtonProps {
  /** Button icon */
  icon: ReactNode;
  /** Button label */
  label: string;
  /** Whether the button is active */
  active?: boolean;
  /** Click handler */
  onClick: () => void;
  /** Additional sx props */
  sx?: SxProps<Theme>;
  /** Tooltip placement */
  tooltipPlacement?: 'bottom' | 'top' | 'left' | 'right';
}

/**
 * Zen navigation button component
 *
 * Used in the zen navigation bar for quick access to key features.
 * Features a square layout with icon above label.
 *
 * @example
 * ```tsx
 * <ZenButton
 *   icon={<Claude size={22} />}
 *   label="Claude"
 *   active={true}
 *   onClick={() => navigate('/use-claude-code')}
 * />
 * ```
 */
export const ZenButton: React.FC<ZenButtonProps> = ({
  icon,
  label,
  active = false,
  onClick,
  sx,
  tooltipPlacement = 'bottom',
}) => {
  return (
    <Tooltip title={label} placement={tooltipPlacement} arrow>
      <IconButton
        onClick={onClick}
        sx={{
          width: 44,
          height: 44,
          flexDirection: 'column',
          gap: 0.25,
          color: active ? 'primary.contrastText' : 'text.secondary',
          bgcolor: active ? 'primary.main' : 'transparent',
          borderRadius: 2,
          transition: 'background-color 0.18s ease-out, color 0.18s ease-out',
          '&:hover': {
            bgcolor: active ? 'primary.main' : 'action.hover',
          },
          ...sx,
        }}
        aria-label={label}
        aria-pressed={active}
      >
        {icon}
        <Typography
          variant="caption"
          sx={{
            fontSize: '0.65rem',
            fontWeight: active ? 600 : 400,
            lineHeight: 1.2,
            textAlign: 'center',
            maxWidth: '100%',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}
        >
          {label}
        </Typography>
      </IconButton>
    </Tooltip>
  );
};

export default ZenButton;
