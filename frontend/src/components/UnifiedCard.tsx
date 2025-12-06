import { Alert, Box, Card, CardContent, IconButton, Tooltip, Typography, Fade } from '@mui/material';
import { Pause, PlayArrow, Refresh, KeyboardArrowUp, KeyboardArrowDown } from '@mui/icons-material';
import type { SxProps, Theme } from '@mui/material/styles';
import type { ReactNode } from 'react';
import { useEffect, useRef, useState, useCallback } from 'react';

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
  // Enhanced scrolling options
  showScrollIndicator?: boolean;
  showScrollButtons?: boolean;
  scrollThrottle?: boolean;
  enableSmoothScroll?: boolean;
}

// 基本格子尺寸单位（像素）
const BASE_UNIT = 40;

// Auto-scroll speed configuration
const scrollSpeeds = {
  slow: 50, // pixels per second
  medium: 100,
  fast: 200,
};

// Scroll throttling utility
const throttle = <T extends (...args: any[]) => void>(
  func: T,
  delay: number
): ((...args: Parameters<T>) => void) => {
  let timeoutId: number | null = null;
  let lastExecTime = 0;

  return (...args: Parameters<T>) => {
    const currentTime = Date.now();

    if (currentTime - lastExecTime > delay) {
      func(...args);
      lastExecTime = currentTime;
    } else {
      if (timeoutId) {
        clearTimeout(timeoutId);
      }
      timeoutId = setTimeout(() => {
        func(...args);
        lastExecTime = Date.now();
        timeoutId = null;
      }, delay - (currentTime - lastExecTime)) as unknown as number;
    }
  };
};

// 预设的格子倍数系统 - 使用相对尺寸和自适应布局
const presetCardDimensions = {
  small: {
    width: '25%',  // 25% 宽度（相对于父容器）
    height: '50%', // 50% 高度（相对于父容器）
    hasFixedHeight: true,
  },
  medium: {
    width: '50%',  // 50% 宽度（相对于父容器）
    height: '50%', // 50% 高度（相对于父容器）
    hasFixedHeight: true,
  },
  large: {
    width: '100%', // 自适应父容器最大宽度
    minHeightUnits: 10, // 最小高度 400px
    hasFixedHeight: false,
  },
  full: {
    width: '100%', // 自适应父容器最大宽度
    height: '100%', // 自适应父容器最大高度
    hasFixedHeight: true,
  },
  header: {
    width: '100%', // 自适应父容器最大宽度
    minHeightUnits: 4, // 最小高度 320px
    hasFixedHeight: false,
  },
};

// 计算卡片尺寸的函数
const getCardDimensions = (
  size: 'small' | 'medium' | 'large' | 'full' | 'header',
  customGridUnits?: { widthUnits?: number; heightUnits?: number },
  customWidth?: number | string,
  customHeight?: number | string
) => {
  const preset = presetCardDimensions[size] as any;

  // 如果提供了自定义宽度，优先使用自定义宽度
  const width = customWidth !== undefined
    ? customWidth
    : customGridUnits?.widthUnits
      ? customGridUnits.widthUnits * BASE_UNIT
      : preset.width;

  // 如果提供了自定义高度，优先使用自定义高度
  let height: string | number;
  if (customHeight !== undefined) {
    height = customHeight;
  } else if (customGridUnits?.heightUnits) {
    height = customGridUnits.heightUnits * BASE_UNIT;
  } else {
    // 根据预设尺寸设置高度
    switch (size) {
      case 'small':
      case 'medium':
      case 'full':
        height = preset.height;
        break;
      case 'large':
      case 'header':
        height = preset.minHeightUnits ? preset.minHeightUnits * BASE_UNIT : 'auto';
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
  showScrollIndicator = true,
  showScrollButtons = true,
  scrollThrottle = true,
  enableSmoothScroll = true,
}: UnifiedCardProps) => {
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const animationRef = useRef<number | undefined>();
  const lastTimeRef = useRef<number>(0);
  const [showScrollTop, setShowScrollTop] = useState(false);
  const [showScrollBottom, setShowScrollBottom] = useState(false);
  const [isScrollable, setIsScrollable] = useState(false);

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

  // Check scrollability and update scroll indicators
  const checkScrollability = useCallback(() => {
    const container = scrollContainerRef.current;
    if (!container) return;

    const hasVerticalScroll = container.scrollHeight > container.clientHeight;
    const hasHorizontalScroll = container.scrollWidth > container.clientWidth;
    const isScrollableContent = hasVerticalScroll || hasHorizontalScroll;

    setIsScrollable(isScrollableContent);

    // Update scroll button visibility
    if (showScrollButtons && hasVerticalScroll) {
      setShowScrollTop(container.scrollTop > 50);
      setShowScrollBottom(container.scrollTop < container.scrollHeight - container.clientHeight - 50);
    }
  }, [showScrollButtons]);

  // Throttled scroll handler
  const handleScroll = useCallback(
    scrollThrottle
      ? throttle(checkScrollability, 16) // ~60fps
      : checkScrollability,
    [scrollThrottle, checkScrollability]
  );

  // Monitor scroll changes
  useEffect(() => {
    const container = scrollContainerRef.current;
    if (!container) return;

    // Initial check
    checkScrollability();

    // Add scroll listener
    container.addEventListener('scroll', handleScroll, { passive: true });

    // Resize observer for content changes
    const resizeObserver = new ResizeObserver(() => {
      checkScrollability();
    });
    resizeObserver.observe(container);
    resizeObserver.observe(container.firstElementChild || container);

    return () => {
      container.removeEventListener('scroll', handleScroll);
      resizeObserver.disconnect();
    };
  }, [handleScroll, checkScrollability]);

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

  const scrollToTop = () => {
    if (scrollContainerRef.current) {
      scrollContainerRef.current.scrollTo({
        top: 0,
        behavior: enableSmoothScroll ? 'smooth' : 'auto'
      });
    }
  };

  const scrollToBottom = () => {
    if (scrollContainerRef.current) {
      const container = scrollContainerRef.current;
      container.scrollTo({
        top: container.scrollHeight,
        behavior: enableSmoothScroll ? 'smooth' : 'auto'
      });
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
                scrollBehavior: enableSmoothScroll ? 'smooth' : 'auto',
                height: scrollContentHeight || '100%',
                position: 'relative',
                // Enhanced scrollbar styling
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
                  transition: 'background 0.2s ease',
                },
                '&::-webkit-scrollbar-thumb:hover': {
                  background: 'rgba(0, 0, 0, 0.4)',
                },
                '&::-webkit-scrollbar-corner': {
                  background: 'transparent',
                },
                // Firefox scrollbar styling
                scrollbarWidth: 'thin',
                scrollbarColor: 'rgba(0, 0, 0, 0.2) rgba(0, 0, 0, 0.05)',
              }}
              onClick={handleScrollToggle}
              style={{ cursor: onScrollToggle ? 'pointer' : 'default' }}
            >
              {children}
              {/* Scroll indicators */}
              {showScrollIndicator && isScrollable && (
                <>
                  {/* Top scroll indicator */}
                  <Fade in={showScrollTop}>
                    <Box
                      sx={{
                        position: 'absolute',
                        top: 0,
                        left: 0,
                        right: 0,
                        height: '20px',
                        background: 'linear-gradient(to bottom, rgba(0,0,0,0.1), transparent)',
                        pointerEvents: 'none',
                        zIndex: 1,
                      }}
                    />
                  </Fade>
                  {/* Bottom scroll indicator */}
                  <Fade in={showScrollBottom}>
                    <Box
                      sx={{
                        position: 'absolute',
                        bottom: 0,
                        left: 0,
                        right: 0,
                        height: '20px',
                        background: 'linear-gradient(to top, rgba(0,0,0,0.1), transparent)',
                        pointerEvents: 'none',
                        zIndex: 1,
                      }}
                    />
                  </Fade>
                </>
              )}
              {/* Scroll navigation buttons */}
              {showScrollButtons && isScrollable && (
                <>
                  <Fade in={showScrollTop}>
                    <Tooltip title="Scroll to top">
                      <IconButton
                        size="small"
                        onClick={scrollToTop}
                        sx={{
                          position: 'absolute',
                          top: 8,
                          right: 8,
                          backgroundColor: 'background.paper',
                          boxShadow: 2,
                          zIndex: 2,
                          '&:hover': {
                            backgroundColor: 'background.default',
                          },
                        }}
                      >
                        <KeyboardArrowUp fontSize="small" />
                      </IconButton>
                    </Tooltip>
                  </Fade>
                  <Fade in={showScrollBottom}>
                    <Tooltip title="Scroll to bottom">
                      <IconButton
                        size="small"
                        onClick={scrollToBottom}
                        sx={{
                          position: 'absolute',
                          bottom: 8,
                          right: 8,
                          backgroundColor: 'background.paper',
                          boxShadow: 2,
                          zIndex: 2,
                          '&:hover': {
                            backgroundColor: 'background.default',
                          },
                        }}
                      >
                        <KeyboardArrowDown fontSize="small" />
                      </IconButton>
                    </Tooltip>
                  </Fade>
                </>
              )}
            </Box>
          ) : (
            <Box
              sx={{
                flex: 1,
                display: 'flex',
                flexDirection: 'column',
                position: 'relative',
                overflow: 'auto',
                // Enhanced scrollbar styling for non-auto-scroll mode
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
                  transition: 'background 0.2s ease',
                },
                '&::-webkit-scrollbar-thumb:hover': {
                  background: 'rgba(0, 0, 0, 0.4)',
                },
                scrollbarWidth: 'thin',
                scrollbarColor: 'rgba(0, 0, 0, 0.2) rgba(0, 0, 0, 0.05)',
              }}
            >
              {children}
            </Box>
          )}
        </Box>
      </CardContent>
    </Card>
  );
};

export default UnifiedCard;