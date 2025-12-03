import { Alert, Box, Card, CardContent, Typography } from '@mui/material';
import type { SxProps, Theme } from '@mui/material/styles';
import type { ReactNode } from 'react';

interface UnifiedCardProps {
  title?: string;
  subtitle?: string;
  children: ReactNode;
  // 格子倍数配置：widthUnits × heightUnits
  size?: 'small' | 'medium' | 'large' | 'full';
  variant?: 'default' | 'outlined' | 'elevated';
  // 自定义格子倍数
  gridUnits?: {
    widthUnits?: number;
    heightUnits?: number;
  };
  // Message support
  message?: { type: 'success' | 'error'; text: string } | null;
  onClearMessage?: () => void;
  // Header actions
  leftAction?: ReactNode;
  rightAction?: ReactNode;
  sx?: SxProps<Theme>;
}

// 基本格子尺寸单位（像素）
const BASE_UNIT = 40;

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
  fullw:{
        widthUnits: 28, // 520px
    minHeightUnits: 12, // 最小高度 480px
  }
};

// 计算卡片尺寸的函数
const getCardDimensions = (size: 'small' | 'medium' | 'large' | 'full', customGridUnits?: { widthUnits?: number; heightUnits?: number }) => {
  const preset = presetCardDimensions[size];
  const width = (customGridUnits?.widthUnits || preset.widthUnits) * BASE_UNIT;

  // 如果有自定义高度，使用自定义高度，否则使用最小高度
  const height = customGridUnits?.heightUnits
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
  message,
  onClearMessage,
  leftAction,
  rightAction,
  sx = {},
}: UnifiedCardProps) => {
  return (
    <Card
      sx={{
        ...getCardDimensions(size, gridUnits),
        ...cardVariants[variant],
        borderRadius: 2,
        border: '1px solid',
        borderColor: 'divider',
        backgroundColor: 'background.paper',
        transition: 'all 0.2s ease-in-out',
        '&:hover': {
          boxShadow: 2,
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
          overflow: 'hidden',
        }}
      >
        {title && (
          <Box sx={{ mb: 2, flexShrink: 0 }}>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: subtitle ? 1 : 0 }}>
              <Box sx={{ flex: 1 }}>
                <Typography variant="h4" sx={{ fontWeight: 600, color: 'text.primary' }}>
                  {title}
                </Typography>
              </Box>
              <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
                {leftAction}
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
        <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', overflowY: 'auto' }}>
          {children}
        </Box>
      </CardContent>
    </Card>
  );
};

export default UnifiedCard;