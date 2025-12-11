import type { SxProps, Theme } from '@mui/material/styles';

// Common style patterns used throughout the application
export const commonStyles = {
  // Card styles
  card: {
    p: 2,
    border: 1,
    borderColor: 'divider',
    borderRadius: 2,
    bgcolor: 'background.paper',
    transition: 'all 0.2s ease-in-out',
    '&:hover': {
      boxShadow: 2,
    },
  } as SxProps<Theme>,

  cardElevated: {
    p: 2,
    borderRadius: 2,
    boxShadow: 3,
    bgcolor: 'background.paper',
    transition: 'all 0.2s ease-in-out',
    '&:hover': {
      boxShadow: 4,
      transform: 'translateY(-2px)',
    },
  } as SxProps<Theme>,

  // Layout styles
  flexCenter: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
  } as SxProps<Theme>,

  flexBetween: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  } as SxProps<Theme>,

  // Status indicators
  successBorder: {
    borderColor: 'success.main',
    borderWidth: 2,
  } as SxProps<Theme>,

  errorBorder: {
    borderColor: 'error.main',
    borderWidth: 2,
  } as SxProps<Theme>,

  primaryBorder: {
    borderColor: 'primary.main',
    borderWidth: 2,
  } as SxProps<Theme>,

  // Form styles
  formStack: {
    spacing: 2,
    mt: 1,
  } as SxProps<Theme>,

  // Grid layouts
  modelGrid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fill, minmax(140px, 1fr))',
    gap: 1,
  } as SxProps<Theme>,

  providerGrid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))',
    gap: 2,
  } as SxProps<Theme>,

  // Responsive breakpoints
  responsiveContainer: {
    maxWidth: 'lg',
    mx: 'auto',
    px: { xs: 2, sm: 3 },
  } as SxProps<Theme>,

  // Animation styles
  fadeIn: {
    animation: 'fadeIn 0.3s ease-in-out',
    '@keyframes fadeIn': {
      from: { opacity: 0 },
      to: { opacity: 1 },
    },
  } as SxProps<Theme>,

  slideUp: {
    animation: 'slideUp 0.3s ease-out',
    '@keyframes slideUp': {
      from: { transform: 'translateY(20px)', opacity: 0 },
      to: { transform: 'translateY(0)', opacity: 1 },
    },
  } as SxProps<Theme>,
};

// Status color utilities
export const getStatusColor = (enabled: boolean, isDefault?: boolean) => {
  if (isDefault) return 'primary.main';
  return enabled ? 'success.main' : 'error.main';
};

export const getStatusBgColor = (enabled: boolean, isDefault?: boolean) => {
  if (isDefault) return 'primary.50';
  return enabled ? 'success.50' : 'error.50';
};

// Common spacing values
export const spacing = {
  xs: 1,
  sm: 2,
  md: 3,
  lg: 4,
  xl: 5,
};

// Common breakpoints
export const breakpoints = {
  mobile: 'sm',
  tablet: 'md',
  desktop: 'lg',
  wide: 'xl',
};