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

    const isPrimary = priority === 0;

    const tooltipContent = (
        <Box sx={{ py: 0.25 }}>
            <Typography variant="caption" sx={{ fontWeight: 700, display: 'block', mb: 0.5 }}>
                {isPrimary
                    ? t('rule.tier.nodeTooltipPrimaryTitle', { defaultValue: 'T0 — Highest priority' })
                    : t('rule.tier.nodeTooltipFallbackTitle', { tier: priority, prev: priority - 1, defaultValue: `T${priority} — Fallback tier` })
                }
            </Typography>
            <Typography variant="caption" sx={{ display: 'block', color: 'inherit', opacity: 0.85 }}>
                {isPrimary
                    ? t('rule.tier.nodeTooltipPrimaryBody', { defaultValue: 'Tried first on every request. Services here are load-balanced.' })
                    : t('rule.tier.nodeTooltipFallbackBody', { tier: priority, prev: priority - 1, defaultValue: `Only used when all T${priority - 1} services are unavailable. Services here are load-balanced.` })
                }
            </Typography>
            <Typography variant="caption" sx={{ display: 'block', mt: 0.75, opacity: 0.65 }}>
                {t('rule.tier.nodeMoveHint', { defaultValue: '↑ / ↓  on a service card to move it to a different tier' })}
            </Typography>
        </Box>
    );

    return (
        <NodeTooltip title={tooltipContent} placement="left" enterDelay={400}>
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
                    cursor: 'default',
                })}
            >
                <Typography
                    sx={{
                        fontSize: '0.9rem',
                        fontWeight: 700,
                        color: 'text.secondary',
                        letterSpacing: '0.02em',
                        lineHeight: 1,
                    }}
                >
                    {`T${priority}`}
                </Typography>
            </Box>
        </NodeTooltip>
    );
};

/** @deprecated Use TierNode */
export const PriorityTierNode = TierNode;

export default TierNode;
