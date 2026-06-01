import { Box, Typography } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import NodeTooltip from './NodeTooltip.tsx';
import { getRouteGraphBorderColor, PROVIDER_NODE_STYLES } from './styles.tsx';

export interface TierNodeProps {
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

    const title = isPrimary
        ? t('rule.tier.nodeTooltipPrimaryTitle', { defaultValue: 'T0 — Highest priority' })
        : t('rule.tier.nodeTooltipFallbackTitle', { tier: priority, prev: priority - 1, defaultValue: `T${priority} — Fallback tier` });
    const body = isPrimary
        ? t('rule.tier.nodeTooltipPrimaryBody', { defaultValue: 'Tried first on every request. Services here are load-balanced.' })
        : t('rule.tier.nodeTooltipFallbackBody', { tier: priority, prev: priority - 1, defaultValue: 'Tried only when all higher-priority tiers are unavailable (lower number = higher priority). Services here are load-balanced.' });
    const hint = t('rule.tier.nodeMoveHint', { defaultValue: '↑ / ↓  on a service card to move it to a different tier' });

    const tooltipContent = (
        <Box sx={{ whiteSpace: 'pre-line', maxWidth: 240 }}>
            <strong>{title}</strong>
            {`\n${body}\n\n`}
            <Box component="span" sx={{ opacity: 0.7 }}>{hint}</Box>
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
                        fontSize: '0.8rem',
                        fontWeight: 600,
                        color: 'text.secondary',
                        lineHeight: 1.15,
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
