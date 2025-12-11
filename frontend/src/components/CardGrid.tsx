import { Grid } from '@mui/material';
import type { GridProps } from '@mui/material';
import type { ReactNode } from 'react';

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
  ...gridProps
}: CardGridProps) => {
  return (
    <Grid
      container
      spacing={spacing}
      {...gridProps}
    >
      {children}
    </Grid>
  );
};

interface CardGridItemProps {
  children: ReactNode;
  xs?: number;
  sm?: number;
  md?: number;
  lg?: number;
  xl?: number;
}

export const CardGridItem = ({
  children,
  xs = 12,
  sm = 6,
  md = 4,
  lg = 3,
  xl = 3
}: CardGridItemProps) => {
  return (
    <Grid item xs={xs} sm={sm} md={md} lg={lg} xl={xl}>
      {children}
    </Grid>
  );
};

export default CardGrid;