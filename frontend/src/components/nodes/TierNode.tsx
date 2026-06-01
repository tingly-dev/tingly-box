import { Box, Typography } from '@mui/material';
import { alpha } from '@mui/material/styles';
import React from 'react';
import { useTranslation } from 'react-i18next';
import NodeTooltip from './NodeTooltip.tsx';
import { getRouteGraphActiveColor, PROVIDER_NODE_STYLES } from './styles.tsx';

export interface TierNodeProps {
    tierIndex: number;
    priority: number;
    active: boolean;
}

export const TIER_NODE_WIDTH = 52;

/** @deprecated Use TIER_NODE_WIDTH */
export const PRIORITY_TIER_NODE_WIDTH = TIER_NODE_WIDTH;

export const TierNode: React.FC<TierNodeProps> = ({
    priority,
    active,
}) => {
    const { t } = useTranslation();

    return (
        <Box
            sx={(theme) => ({
                width: TIER_NODE_WIDTH,
                height: PROVIDER_NODE_STYLES.height,
                flexShrink: 0,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                borderRadius: `${theme.shape.borderRadius}px`,
                border: '1px solid',
                borderColor: alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.44 : 0.38),
                bgcolor: alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.07 : 0.04),
                opacity: active ? 1 : 0.6,
                transition: 'border-color 0.16s, opacity 0.16s',
                userSelect: 'none',
            })}
        >
            <NodeTooltip title={t('rule.tier.tooltip')} placement="left" arrow>
                <Typography
                    sx={(theme) => ({
                        fontSize: '0.95rem',
                        fontWeight: 800,
                        color: getRouteGraphActiveColor(theme),
                        letterSpacing: '0.02em',
                        lineHeight: 1,
                        cursor: 'default',
                    })}
                >
                    {`T${priority}`}
                </Typography>
            </NodeTooltip>
        </Box>
    );
};

/** @deprecated Use TierNode */
export const PriorityTierNode = TierNode;

export default TierNode;
