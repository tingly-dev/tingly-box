import { Pause, PlayArrow, Refresh } from '@mui/icons-material';
import { Alert, Box, Card, CardContent, IconButton, Tooltip, Typography } from '@mui/material';
import type { SxProps, Theme } from '@mui/material/styles';
import type { ReactNode } from 'react';
import { useEffect, useRef } from 'react';

interface UnifiedCardProps {
  title?: string;
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
  // Auto-scroll functionality
  autoScroll?: boolean;
  scrollSpeed?: 'slow' | 'medium' | 'fast';
  scrollDirection?: 'up' | 'down' | 'left' | 'right';
  scrollPaused?: boolean;
  scrollContentHeight?: number;
  onScrollToggle?: (paused: boolean) => void;
  // Enhanced scrolling options
  showScrollIndicator?: boolean;
  showScrollButtons?: boolean;
  scrollThrottle?: boolean;
  enableSmoothScroll?: boolean;
}


// Auto-scroll speed configuration
const scrollSpeeds = {
  slow: 50, // pixels per second
  medium: 100,
  fast: 200,
};

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
    hasFixedHeight: true,
  },
  full: {
    width: '100%', // Adaptive to parent container max width
    height: '100%', // Adaptive to parent container max height
    hasFixedHeight: true,
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

  // If custom height is provided, prioritize using custom height
  let height: string | number;
  if (customHeight !== undefined) {
    height = customHeight;
  } else {
    // Set height based on preset size
    switch (size) {
      case 'small':
      case 'medium':
      case 'full':
        height = preset.height;
        break;
      case 'large':
      case 'header':
        height = preset.minHeight || 'auto';
        break;
      default:
        height = 'auto';
    }
  }

  return {
    width,
    height,
    display: 'flex',
    flexDirection: 'column' as const,
  };
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
  autoScroll = false,
  scrollSpeed = 'medium',
  scrollDirection = 'down',
  scrollPaused = false,
  scrollContentHeight,
  onScrollToggle,
}: UnifiedCardProps) => {
  const contentRef = useRef<HTMLDivElement>(null);

  // Auto-scroll implementation (kept for backward compatibility)
  useEffect(() => {
    if (!autoScroll || scrollPaused || !contentRef.current) return;

    const container = contentRef.current;
    const speed = scrollSpeeds[scrollSpeed];
    let animationId: number;

    const animate = () => {
      switch (scrollDirection) {
        case 'down':
          if (container.scrollHeight - container.scrollTop - container.clientHeight > 1) {
            container.scrollTop += speed / 60; // 60fps approximation
          } else {
            container.scrollTop = 0; // Reset to top
          }
          break;
        case 'up':
          if (container.scrollTop > 0) {
            container.scrollTop -= speed / 60;
          } else {
            container.scrollTop = container.scrollHeight - container.clientHeight;
          }
          break;
      }
      animationId = requestAnimationFrame(animate);
    };

    animationId = requestAnimationFrame(animate);

    return () => {
      cancelAnimationFrame(animationId);
    };
  }, [autoScroll, scrollPaused, scrollSpeed, scrollDirection]);

  const handleScrollToggle = () => {
    onScrollToggle?.(!scrollPaused);
  };
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
                {autoScroll && (
                  <>
                    <Tooltip title={scrollPaused ? 'Resume scrolling' : 'Pause scrolling'}>
                      <IconButton
                        size="small"
                        onClick={handleScrollToggle}
                        sx={{ color: 'text.secondary' }}
                      >
                        {scrollPaused ? <PlayArrow /> : <Pause />}
                      </IconButton>
                    </Tooltip>
                    <Tooltip title="Reset scroll position">
                      <IconButton
                        size="small"
                        onClick={() => {
                          if (contentRef.current) {
                            contentRef.current.scrollTop = 0;
                          }
                        }}
                        sx={{ color: 'text.secondary' }}
                      >
                        <Refresh />
                      </IconButton>
                    </Tooltip>
                  </>
                )}
                {rightAction}
              </Box>
            </Box>
            {subtitle && (
              <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                {subtitle}
              </Typography>
            )}
            {autoScroll && (
              <Typography variant="caption" sx={{ color: 'text.secondary', display: 'flex', alignItems: 'center', gap: 0.5 }}>
                <Box
                  sx={{
                    width: 8,
                    height: 8,
                    borderRadius: '50%',
                    backgroundColor: scrollPaused ? 'warning.main' : 'success.main',
                    animation: !scrollPaused ? 'pulse 2s infinite' : 'none',
                  }}
                />
                {scrollPaused ? 'Auto-scroll paused' : `Auto-scrolling ${scrollDirection} (${scrollSpeed})`}
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
            ref={contentRef}
            sx={{
              flex: 1,
              overflow: 'auto',
              height: scrollContentHeight || '100%',
              position: 'relative',
              // Simple scrollbar styling
              '&::-webkit-scrollbar': {
                width: '8px',
                height: '8px',
              },
              '&::-webkit-scrollbar-track': {
                background: 'rgba(0, 0, 0, 0.05)',
                borderRadius: '4px',
              },
              '&::-webkit-scrollbar-thumb': {
                background: 'rgba(0, 0, 0, 0.2)',
                borderRadius: '4px',
              },
              '&::-webkit-scrollbar-thumb:hover': {
                background: 'rgba(0, 0, 0, 0.3)',
              },
              // Firefox scrollbar
              scrollbarWidth: 'thin',
              scrollbarColor: 'rgba(0, 0, 0, 0.2) transparent',
            }}
            onClick={autoScroll ? handleScrollToggle : undefined}
            style={{ cursor: autoScroll && onScrollToggle ? 'pointer' : 'default' }}
          >
            {children}
          </Box>
        </Box>
      </CardContent>
    </Card>
  );
};

export default UnifiedCard;