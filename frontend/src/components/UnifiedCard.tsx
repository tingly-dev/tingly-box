import { Alert, Box, Card, CardContent, Typography } from '@mui/material';
import type { SxProps, Theme } from '@mui/material/styles';
import type { ReactNode } from 'react';

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
}


// Preset size configuration - using relative sizes and responsive layout
interface PresetDimensions {
  width: string;
  height?: string;
  minHeight?: string;
  hasFixedHeight: boolean;
}

const presetCardDimensions: Record<string, PresetDimensions> = {
  small: {
    width: '25%',  // 25% width (relative to parent container)
    height: '50%', // 50% height (relative to parent container)
    hasFixedHeight: true,
  },
  medium: {
    width: '50%',  // 50% width (relative to parent container)
    height: '100%', // 100% height (relative to parent container)
    hasFixedHeight: true,
  },
  large: {
    width: '100%', // Adaptive to parent container max width
    minHeight: '400px', // Min height 400px
    hasFixedHeight: false,
  },
  full: {
    width: '100%', // Adaptive to parent container max width
    // No height constraints - let content determine height
    hasFixedHeight: false,
  },
  header: {
    width: '100%', // Adaptive to parent container max width
    minHeight: '200px', // Min height 200px
    hasFixedHeight: false,
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

  // Determine height/minHeight based on preset and custom values
  const dimensions: any = {
    width,
    display: 'flex',
    flexDirection: 'column' as const,
  };

  if (customHeight !== undefined) {
    dimensions.height = customHeight;
  } else if (preset.hasFixedHeight && preset.height) {
    dimensions.height = preset.height;
  } else if (preset.minHeight) {
    dimensions.minHeight = preset.minHeight;
  }

  return dimensions;
};

const cardVariants = {
  default: {},
  outlined: {
    border: 2,
    borderColor: 'divider',
    boxShadow: 'none',
  },
  elevated: {
    boxShadow: '0 4px 12px rgba(0, 0, 0, 0.1)',
    border: 'none',
  },
};

export const UnifiedCard = ({
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
}: UnifiedCardProps) => {
  return (
    <Card
      sx={{
        ...getCardDimensions(size, width, height),
        ...cardVariants[variant],
        borderRadius: 2,
        border: '1px solid',
        borderColor: 'divider',
        backgroundColor: 'background.paper',
        transition: 'all 0.2s ease-in-out',
        '&:hover': {
          boxShadow: 2,
        },
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
              <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                {subtitle}
              </Typography>
            )}
          </Box>
        )}
        {message && (
          <Box sx={{ mb: 2, flexShrink: 0 }}>
            <Alert
              severity={message.type}
              onClose={onClearMessage}
            >
              {message.text}
            </Alert>
          </Box>
        )}
        <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', position: 'relative' }}>
          <Box
            sx={{
              flex: 1,
              position: 'relative',
            }}
          >
            {children}
          </Box>
        </Box>
      </CardContent>
    </Card>
  );
};

export default UnifiedCard;