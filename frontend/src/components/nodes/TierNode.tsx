import { Box, Typography } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import NodeTooltip from './NodeTooltip.tsx';
import { getRouteGraphBorderColor, PROVIDER_NODE_STYLES } from './styles.tsx';

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
                borderColor: getRouteGraphBorderColor(theme),
                backgroundColor: theme.palette.background.paper,
                opacity: active ? 1 : 0.6,
                transition: 'border-color 0.16s, opacity 0.16s',
                userSelect: 'none',
            })}
        >
            <NodeTooltip title={t('rule.tier.tooltip')} placement="left" arrow>
                <Typography
                    sx={{
                        fontSize: '0.9rem',
                        fontWeight: 700,
                        color: 'text.secondary',
                        letterSpacing: '0.02em',
                        lineHeight: 1,
                        cursor: 'default',
                    }}
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
