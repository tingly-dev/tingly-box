import { Box, Typography } from '@mui/material';
import type { SxProps, Theme } from '@mui/material/styles';
import type { ReactNode } from 'react';

interface PageHeaderProps {
  title: ReactNode;
  subtitle?: ReactNode;
  icon?: ReactNode;
  actions?: ReactNode;
  sx?: SxProps<Theme>;
}

export default function PageHeader({ title, subtitle, icon, actions, sx }: PageHeaderProps) {
  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: { xs: 'stretch', sm: 'center' },
        justifyContent: 'space-between',
        flexDirection: { xs: 'column', sm: 'row' },
        gap: 2,
        pb: 2.5,
        borderBottom: '1px solid',
        borderColor: 'divider',
        ...sx,
      }}
    >
      <Box sx={{ minWidth: 0 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.25 }}>
          {icon && (
            <Box
              aria-hidden="true"
              sx={{
                display: 'flex',
                alignItems: 'center',
                color: 'text.primary',
                flexShrink: 0,
              }}
            >
              {icon}
            </Box>
          )}
          <Typography component="h1" variant="h3" sx={{ color: 'text.primary' }}>
            {title}
          </Typography>
        </Box>
        {subtitle && (
          <Typography
            variant="body2"
            sx={{
              mt: 0.5,
              color: 'text.secondary',
              maxWidth: '72ch',
            }}
          >
            {subtitle}
          </Typography>
        )}
      </Box>
      {actions && (
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: { xs: 'flex-start', sm: 'flex-end' },
            gap: 1.5,
            flexWrap: 'wrap',
          }}
        >
          {actions}
        </Box>
      )}
    </Box>
  );
}
