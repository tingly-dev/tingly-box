import { Alert, Box, Card, CardContent, IconButton, Tooltip, Typography } from '@mui/material';
import { Pause, PlayArrow, Refresh } from '@mui/icons-material';
import type { SxProps, Theme } from '@mui/material/styles';
import type { ReactNode } from 'react';
import { useEffect, useRef } from 'react';

interface UnifiedCardProps {
  title?: string;
  subtitle?: string;
  children: ReactNode;
  // 格子倍数配置：widthUnits × heightUnits
  size?: 'small' | 'medium' | 'large' | 'full' | 'header';
  variant?: 'default' | 'outlined' | 'elevated';
  // 自定义格子倍数
  gridUnits?: {
    widthUnits?: number;
    heightUnits?: number;
  };
  // 自定义宽度，如果提供则优先使用
  width?: number | string;
  // 自定义高度，如果提供则优先使用
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
}

// 基本格子尺寸单位（像素）
const BASE_UNIT = 40;

// Auto-scroll speed configuration
const scrollSpeeds = {
  slow: 50, // pixels per second
  medium: 100,
  fast: 200,
};

// 预设的格子倍数系统 - 使用最小高度而不是固定高度
const presetCardDimensions = {
  small: {
    widthUnits: 8,  // 320px
    minHeightUnits: 6, // 最小高度 240px
  },
  medium: {
    widthUnits: 8,  // 320px
    minHeightUnits: 8, // 最小高度 320px
  },
  large: {
    widthUnits: 12,  // 520px
    minHeightUnits: 10, // 最小高度 400px
  },
  full: {
    widthUnits: 28, // 520px
    minHeightUnits: 24, // 最小高度 480px
  },
  header: {
    widthUnits: 28, // 520px
    minHeightUnits: 4, // 最小高度 320px
  },
  fullw:{
    widthUnits: 28, // 520px
    minHeightUnits: 12, // 最小高度 480px
  }
};

// 计算卡片尺寸的函数
const getCardDimensions = (
  size: 'small' | 'medium' | 'large' | 'full' | 'header',
  customGridUnits?: { widthUnits?: number; heightUnits?: number },
  customWidth?: number | string,
  customHeight?: number | string
) => {
  const preset = presetCardDimensions[size];

  // 如果提供了自定义宽度，优先使用自定义宽度
  const width = customWidth !== undefined
    ? customWidth
    : (customGridUnits?.widthUnits || preset.widthUnits) * BASE_UNIT;

  // 如果提供了自定义高度，优先使用自定义高度
  const height = customHeight !== undefined
    ? customHeight
    : customGridUnits?.heightUnits
      ? customGridUnits.heightUnits * BASE_UNIT
      : preset.minHeightUnits * BASE_UNIT;

  return {
    width,
    height: height,
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
  gridUnits,
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
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const animationRef = useRef<number | undefined>();
  const lastTimeRef = useRef<number>(0);

  useEffect(() => {
    if (!autoScroll || scrollPaused || !scrollContainerRef.current) {
      if (animationRef.current) {
        cancelAnimationFrame(animationRef.current);
      }
      return;
    }

    const container = scrollContainerRef.current;
    const speed = scrollSpeeds[scrollSpeed];

    const animate = (currentTime: number) => {
      if (!lastTimeRef.current) {
        lastTimeRef.current = currentTime;
      }

      const deltaTime = (currentTime - lastTimeRef.current) / 1000; // Convert to seconds
      lastTimeRef.current = currentTime;

      const scrollAmount = speed * deltaTime;

      switch (scrollDirection) {
        case 'down':
          if (container.scrollHeight - container.scrollTop - container.clientHeight > 1) {
            container.scrollTop += scrollAmount;
          } else {
            container.scrollTop = 0; // Reset to top
          }
          break;
        case 'up':
          if (container.scrollTop > 0) {
            container.scrollTop -= scrollAmount;
          } else {
            container.scrollTop = container.scrollHeight - container.clientHeight; // Reset to bottom
          }
          break;
        case 'right':
          if (container.scrollWidth - container.scrollLeft - container.clientWidth > 1) {
            container.scrollLeft += scrollAmount;
          } else {
            container.scrollLeft = 0; // Reset to left
          }
          break;
        case 'left':
          if (container.scrollLeft > 0) {
            container.scrollLeft -= scrollAmount;
          } else {
            container.scrollLeft = container.scrollWidth - container.clientWidth; // Reset to right
          }
          break;
      }

      animationRef.current = requestAnimationFrame(animate);
    };

    animationRef.current = requestAnimationFrame(animate);

    return () => {
      if (animationRef.current) {
        cancelAnimationFrame(animationRef.current);
      }
    };
  }, [autoScroll, scrollPaused, scrollSpeed, scrollDirection]);

  const handleScrollToggle = () => {
    onScrollToggle?.(!scrollPaused);
  };

  const handleResetScroll = () => {
    if (scrollContainerRef.current) {
      const container = scrollContainerRef.current;
      switch (scrollDirection) {
        case 'up':
        case 'down':
          container.scrollTop = 0;
          break;
        case 'left':
        case 'right':
          container.scrollLeft = 0;
          break;
      }
    }
  };
  return (
    <Card
      sx={{
        ...getCardDimensions(size, gridUnits, width, height),
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
                        onClick={handleResetScroll}
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
          {autoScroll ? (
            <Box
              ref={scrollContainerRef}
              sx={{
                flex: 1,
                overflow: 'auto',
                scrollBehavior: 'smooth',
                height: scrollContentHeight || '100%',
                '&::-webkit-scrollbar': {
                  width: '6px',
                  height: '6px',
                },
                '&::-webkit-scrollbar-track': {
                  background: 'transparent',
                },
                '&::-webkit-scrollbar-thumb': {
                  background: 'rgba(0, 0, 0, 0.3)',
                  borderRadius: '3px',
                },
                '&::-webkit-scrollbar-thumb:hover': {
                  background: 'rgba(0, 0, 0, 0.5)',
                },
              }}
              onClick={handleScrollToggle}
              style={{ cursor: onScrollToggle ? 'pointer' : 'default' }}
            >
              {children}
            </Box>
          ) : (
            <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
              {children}
            </Box>
          )}
        </Box>
      </CardContent>
    </Card>
  );
};

export default UnifiedCard;