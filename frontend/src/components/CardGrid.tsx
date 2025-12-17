import { Grid, Box } from '@mui/material';
import type { GridProps } from '@mui/material';
import type { ReactNode } from 'react';
import { useMemo, useRef, useState, useCallback } from 'react';

interface CardGridProps extends Omit<GridProps, 'container'> {
  children: ReactNode;
  columns?: {
    xs?: number;
    sm?: number;
    md?: number;
    lg?: number;
    xl?: number;
  };
  spacing?: number;
  virtualized?: boolean;
  itemHeight?: number;
  containerHeight?: number;
  overscan?: number;
}

const defaultColumns = {
  xs: 12,
  sm: 6,
  md: 4,
  lg: 3,
  xl: 3,
};

export const CardGrid = ({
  children,
  columns = defaultColumns,
  spacing = 3,
  virtualized = false,
  itemHeight = 300,
  containerHeight: propContainerHeight = 600,
  overscan = 5,
  ...gridProps
}: CardGridProps) => {
  // If virtualization is not enabled, render normally
  if (!virtualized) {
    return (
      <Grid
        container
        spacing={spacing}
        {...gridProps}
      >
        {children}
      </Grid>
    );
  }

  // Enhanced virtualization implementation
  const [scrollTop, setScrollTop] = useState(0);
  const containerRef = useRef<HTMLDivElement>(null);
  const childrenArray = useMemo(() => {
    const filtered = Array.isArray(children) ? children.filter(Boolean) : [children].filter(Boolean);
    return filtered;
  }, [children]);

  // Calculate visible items with proper column support
  const visibleItems = useMemo(() => {
    const currentContainerHeight = containerRef.current?.clientHeight || propContainerHeight;
    const spacingValue = spacing * 4; // Convert spacing unit to pixels (approximate)
    const actualItemHeight = itemHeight + spacingValue;

    const startIndex = Math.max(0, Math.floor(scrollTop / actualItemHeight) - overscan);
    const endIndex = Math.min(
      childrenArray.length,
      Math.ceil((scrollTop + currentContainerHeight) / actualItemHeight) + overscan
    );

    return childrenArray.slice(startIndex, endIndex).map((child, index) => ({
      child,
      index: startIndex + index,
      style: {
        position: 'absolute' as const,
        top: (startIndex + index) * actualItemHeight,
        left: spacingValue / 2,
        right: spacingValue / 2,
        height: itemHeight,
      },
    }));
  }, [childrenArray, scrollTop, itemHeight, spacing, overscan, propContainerHeight]);

  const totalHeight = Math.max(0, childrenArray.length * (itemHeight + spacing * 8));

  const handleScroll = useCallback((e: React.UIEvent<HTMLDivElement>) => {
    setScrollTop(e.currentTarget.scrollTop);
  }, []);

  return (
    <Box
      ref={containerRef}
      sx={{
        height: propContainerHeight,
        overflowY: 'auto',
        overflowX: 'hidden',
        scrollBehavior: 'smooth',
        '&::-webkit-scrollbar': {
          width: 8,
        },
        '&::-webkit-scrollbar-track': {
          backgroundColor: 'grey.100',
          borderRadius: 1,
        },
        '&::-webkit-scrollbar-thumb': {
          backgroundColor: 'grey.300',
          borderRadius: 1,
          '&:hover': {
            backgroundColor: 'grey.400',
          },
        },
      }}
      onScroll={handleScroll}
    >
      <Box sx={{ height: totalHeight, position: 'relative' }}>
        {visibleItems.map(({ child, index, style }) => (
          <Box
            key={index}
            sx={style}
          >
            {child}
          </Box>
        ))}
      </Box>
    </Box>
  );
};

interface CardGridItemProps {
  children: ReactNode;
  xs?: number;
  sm?: number;
  md?: number;
  lg?: number;
  xl?: number;
  virtualized?: boolean;
  itemHeight?: number;
  isVisible?: boolean;
}

export const CardGridItem = ({
  children,
  xs = 12,
  sm = 6,
  md = 4,
  lg = 3,
  xl = 3,
  virtualized = false,
  itemHeight = 300,
  isVisible = true,
}: CardGridItemProps) => {
  if (virtualized) {
    return (
      <Box
        sx={{
          height: itemHeight,
          display: isVisible ? 'block' : 'none',
        }}
      >
        {children}
      </Box>
    );
  }

  return (
    <Grid item xs={xs} sm={sm} md={md} lg={lg} xl={xl}>
      {children}
    </Grid>
  );
};

export default CardGrid;