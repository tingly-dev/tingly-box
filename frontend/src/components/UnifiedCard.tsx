import { Alert, Box, Card, CardContent, Grid, Typography } from '@mui/material';
import type { SxProps, Theme } from '@mui/material/styles';
import type { ElementType, ReactNode } from 'react';
import { forwardRef } from 'react';
import { EMPTY_SX } from '@/constants/defaults';

/**
 * Responsive column span for a card placed inside a `CardGrid` (MUI Grid v7).
 * Each value is the number of columns (out of the grid's total, default 12)
 * the card occupies at that breakpoint. Omit a breakpoint to let it default.
 * Omit `grid` entirely to keep the legacy "auto" behavior (bare card, stacked).
 */
export interface UnifiedCardGridSpan {
  xs?: number;
  sm?: number;
  md?: number;
  lg?: number;
  xl?: number;
}

interface UnifiedCardProps {
  title?: string | ReactNode;
  titleHeadingLevel?: 1 | 2 | 3 | 4 | 5 | 6;
  /** Space between the header and body; defaults to the standard 2-unit gap. */
  titleMarginBottom?: number | string;
  subtitle?: string;
  children: ReactNode;
  size?: 'small' | 'medium' | 'large' | 'full' | 'header' | 'footer';
  variant?: 'default' | 'outlined' | 'elevated';
  // Custom width, prioritized if provided
  width?: number | string;
  // Custom height, prioritized if provided
  height?: number | string;
  /**
   * Cap the card *content*'s width on wide viewports. The card itself stays
   * full-width (consistent with every other card on the page), but the body
   * stops growing past this value so text lines and form controls don't
   * stretch uncomfortably wide. Content stays left-aligned under the title.
   * Use for settings-style cards with narrow, row-based content.
   */
  contentMaxWidth?: number | string;
  // Message support
  message?: { type: 'success' | 'error'; text: string } | null;
  onClearMessage?: () => void;
  // Header actions
  leftAction?: ReactNode;
  rightAction?: ReactNode;
  sx?: SxProps<Theme>;
  // DOM id forwarded to the root Card — useful as a scroll/anchor target
  id?: string;
  /**
   * Make this card a grid item with the given responsive column span, so it
   * can sit beside other `grid`-enabled cards in an n×m layout inside a
   * `CardGrid`. Default `auto` (omitted) renders a bare card with no grid
   * wrapping — the legacy stacked-full-width behavior.
   */
  grid?: UnifiedCardGridSpan;
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
  },
};

// Function to calculate card dimensions
const getCardDimensions = (
  size: 'small' | 'medium' | 'large' | 'full' | 'header' | 'footer',
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
  titleHeadingLevel = 2,
  titleMarginBottom = 2,
  subtitle,
  children,
  size = 'medium',
  variant = 'default',
  width,
  height,
  contentMaxWidth,
  message,
  onClearMessage,
  leftAction,
  rightAction,
  sx = EMPTY_SX,
  id,
  grid,
}, ref) => {
  const card = (
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
          <Box sx={{ mb: titleMarginBottom, flexShrink: 0 }}>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: subtitle ? 1 : 0 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1 }}>
                {typeof title === 'string' || typeof title === 'number' ? (
                  <Typography
                    component={`h${titleHeadingLevel}` as ElementType}
                    variant="h4"
                    sx={{ fontWeight: 600, color: 'text.primary' }}
                  >
                    {title}
                  </Typography>
                ) : (
                  <Box
                    role="heading"
                    aria-level={titleHeadingLevel}
                    sx={{
                      typography: 'h4',
                      fontWeight: 600,
                      color: 'text.primary',
                    }}
                  >
                    {title}
                  </Box>
                )}
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
              ...(contentMaxWidth !== undefined && {
                maxWidth: contentMaxWidth,
              }),
            }}
          >
            {children}
          </Box>
        </Box>
      </CardContent>
    </Card>
  );

  // When `grid` is provided, become a responsive Grid item so the card can sit
  // beside other grid-enabled cards in an n×m layout. Omit `grid` for the
  // legacy "auto" stacked-full-width behavior.
  return grid ? <Grid size={grid}>{card}</Grid> : card;
});

UnifiedCard.displayName = 'UnifiedCard';

export default UnifiedCard;
