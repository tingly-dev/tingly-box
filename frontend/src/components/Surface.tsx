import { Box } from '@mui/material';
import type { SxProps, Theme } from '@mui/material/styles';
import type { ReactNode } from 'react';

interface SurfaceProps {
  children: ReactNode;
  variant?: 'plain' | 'outlined' | 'soft';
  padding?: number | string | { xs?: number | string; sm?: number | string; md?: number | string };
  sx?: SxProps<Theme>;
}

export default function Surface({
  children,
  variant = 'outlined',
  padding = { xs: 2, sm: 2.5 },
  sx,
}: SurfaceProps) {
  const variantSx: Record<NonNullable<SurfaceProps['variant']>, SxProps<Theme>> = {
    plain: {
      bgcolor: 'transparent',
    },
    outlined: {
      bgcolor: 'background.paper',
      border: '1px solid',
      borderColor: 'divider',
    },
    soft: {
      bgcolor: 'action.hover',
      border: '1px solid',
      borderColor: 'divider',
    },
  };

  return (
    <Box
      sx={{
        borderRadius: 2,
        boxShadow: 'none',
        p: padding,
        ...variantSx[variant],
        ...sx,
      }}
    >
      {children}
    </Box>
  );
}
