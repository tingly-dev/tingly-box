import { Alert, Box, Card, CardContent, Typography } from '@mui/material';
import type { SxProps, Theme } from '@mui/material/styles';
import type { ReactNode } from 'react';
import React, { forwardRef } from 'react';

interface UnifiedCardProps {
  title?: string | ReactNode;
  subtitle?: string;
  children: ReactNode;
  size?: 'small' | 'medium' | 'large' | 'full' | 'header';
  variant?: 'default' | 'outlined' | 'elevated';
  // Custom width, prioritized if provided
  width?: number | string;
  // Custom height, prioritized if provided
  height?: number | string;
  // Message support
  message?: { type: 'success' | 'error'; text: string } | null;
  onClearMessage?: () => void;
  // Header actions
  leftAction?: ReactNode;
  rightAction?: ReactNode;
  sx?: SxProps<Theme>;
  // DOM id forwarded to the root Card — useful as a scroll/anchor target
  id?: string;
}


// Preset size configuration. Width and height are content-led by default so
// cards remain predictable inside grids, stacks, and responsive layouts.
interface PresetDimensions {
  width: string;
  minHeight?: string;
}

const presetCardDimensions: Record<string, PresetDimensions> = {
  small: {
    width: '100%',
    minHeight: '160px',
  },
  medium: {
    width: '100%',
    minHeight: '240px',
  },
  large: {
    width: '100%',
    minHeight: '360px',
  },
  full: {
    width: '100%',
  },
  header: {
    width: '100%',
    minHeight: '160px',
  },
};

// Function to calculate card dimensions
const getCardDimensions = (
  size: 'small' | 'medium' | 'large' | 'full' | 'header',
  customWidth?: number | string,
  customHeight?: number | string
) => {
  const preset = presetCardDimensions[size];

  // If custom width is provided, prioritize using custom width
  const width = customWidth !== undefined
    ? customWidth
    : preset.width;

  const dimensions: {
    width: number | string;
    display: string;
    flexDirection: 'column';
    height?: number | string;
    minHeight?: string;
  } = {
    width,
    display: 'flex',
    flexDirection: 'column',
  };

  if (customHeight !== undefined) {
    dimensions.height = customHeight;
  } else if (preset.minHeight) {
    dimensions.minHeight = preset.minHeight;
  }

  return dimensions;
};

const cardVariants = {
  default: {},
  outlined: {
    borderColor: 'divider',
    boxShadow: 'none',
  },
  elevated: {
    boxShadow: '0 8px 24px rgba(15, 23, 42, 0.10)',
    border: 'none',
  },
};

export const UnifiedCard = forwardRef<HTMLDivElement, UnifiedCardProps>(({
  title,
  subtitle,
  children,
  size = 'medium',
  variant = 'default',
  width,
  height,
  message,
  onClearMessage,
  leftAction,
  rightAction,
  sx = {},
  id,
}, ref) => {
  return (
    <Card
      ref={ref}
      id={id}
      sx={{
        ...getCardDimensions(size, width, height),
        ...cardVariants[variant],
        borderRadius: 2,
        border: '1px solid',
        borderColor: 'divider',
        backgroundColor: 'background.paper',
        boxShadow: variant === 'elevated' ? undefined : 'none',
        transition: 'box-shadow 0.18s ease-out, border-color 0.18s ease-out',
        '@keyframes pulse': {
          '0%': { opacity: 1 },
          '50%': { opacity: 0.5 },
          '100%': { opacity: 1 },
        },
        ...sx,
      }}
    >
      <CardContent
        sx={{
          display: 'flex',
          flexDirection: 'column',
          p: 3,
          height: '100%',
        }}
      >
        {title && (
          <Box sx={{ mb: 2, flexShrink: 0 }}>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: subtitle ? 1 : 0 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1 }}>
                <Typography variant="h4" sx={{ fontWeight: 600, color: 'text.primary' }}>
                  {title}
                </Typography>
                {leftAction}
              </Box>
              <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
                {rightAction}
              </Box>
            </Box>
            {subtitle && (
              <Typography
                variant="body2"
                sx={{
                  color: 'text.secondary',
                  maxWidth: '800px',
                  display: '-webkit-box',
                  WebkitLineClamp: 2,
                  WebkitBoxOrient: 'vertical',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  lineHeight: 1.5,
                }}
              >
                {subtitle}
              </Typography>
            )}
          </Box>
        )}
        {message && (
          <Box sx={{ mb: 1, flexShrink: 0 }}>
            <Alert
              severity={message.type}
              onClose={onClearMessage}
            >
              {message.text}
            </Alert>
          </Box>
        )}
        <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0 }}>
          <Box
            sx={{
              flex: 1,
              minHeight: 0,
              position: 'relative',
            }}
          >
            {children}
          </Box>
        </Box>
      </CardContent>
    </Card>
  );
});

UnifiedCard.displayName = 'UnifiedCard';

export default UnifiedCard;
