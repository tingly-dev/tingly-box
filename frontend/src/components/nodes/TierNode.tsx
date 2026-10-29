import { Box, Typography } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import NodeTooltip from './NodeTooltip.tsx';
import { getRouteGraphBorderColor, PROVIDER_NODE_STYLES } from './styles.tsx';
import { fontSizes } from '@/theme/fonts';

export interface TierNodeProps {
    priority: number;
    active: boolean;
    onHover?: (hovering: boolean) => void;
    onShowGuide?: () => void;
    /** Last tier only: on hover the node shows a small dashed + at its bottom
     * that adds a tier below. */
    onAddTier?: () => void;
    addTierLabel?: string;
}

export const TIER_NODE_WIDTH = 52;

/** @deprecated Use TIER_NODE_WIDTH */
export const PRIORITY_TIER_NODE_WIDTH = TIER_NODE_WIDTH;

export const TierNode: React.FC<TierNodeProps> = ({
    priority,
    active,
    onHover,
    onShowGuide,
    onAddTier,
    addTierLabel,
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
    const learnMoreLink = onShowGuide ? t('rule.tier.nodeTooltipLearnMore', { defaultValue: 'Learn more about tiers' }) : null;

    const tooltipContent = (
        <Box sx={{ whiteSpace: 'pre-line', maxWidth: 240 }}>
            <strong>{title}</strong>
            {`\n${body}\n\n`}
            <Box component="span" sx={{ opacity: 0.7 }}>{hint}</Box>
            {learnMoreLink && (
                <Box
                    component="span"
                    onClick={(e) => { e.stopPropagation(); if (onShowGuide) onShowGuide(); }}
                    sx={{
                        display: 'block',
                        mt: 1,
                        color: 'primary.main',
                        cursor: 'pointer',
                        fontWeight: 500,
                        '&:hover': { textDecoration: 'underline' }
                    }}
                >
                    {learnMoreLink}
                </Box>
            )}
        </Box>
    );

    return (
        <NodeTooltip title={tooltipContent} placement="left" enterDelay={400}>
            <Box
                className={`tier-node-${priority}`}
                sx={(theme) => ({
                    width: TIER_NODE_WIDTH,
                    height: PROVIDER_NODE_STYLES.height,
                    flexShrink: 0,
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    gap: 0.5,
                    borderRadius: `${theme.shape.borderRadius}px`,
                    border: '1px solid',
                    borderColor: getRouteGraphBorderColor(theme),
                    backgroundColor: theme.palette.background.paper,
                    opacity: active ? 1 : 0.6,
                    transition: 'border-color 0.16s, opacity 0.16s',
                    userSelect: 'none',
                    cursor: 'default',
                    position: 'relative',
                    ...(onAddTier && {
                        '&:hover .tier-node-add': { opacity: 1 },
                        '&:hover .tier-node-label': { transform: 'translateY(-9px)' },
                    }),
                })}
                onMouseEnter={() => onHover?.(true)}
                onMouseLeave={() => onHover?.(false)}
            >
                <Typography
                    className="tier-node-label"
                    sx={{
                        transition: 'transform 0.16s ease',
                        fontSize: fontSizes.md,
                        fontWeight: 600,
                        color: 'text.secondary',
                        lineHeight: 1.15,
                    }}
                >
                    {`T${priority}`}
                </Typography>
                {onAddTier && (
                    <Box
                        className="tier-node-add"
                        role="button"
                        tabIndex={0}
                        title={addTierLabel}
                        aria-label={addTierLabel}
                        onClick={(e) => { e.stopPropagation(); onAddTier(); }}
                        onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onAddTier(); } }}
                        sx={(theme) => ({
                            position: 'absolute',
                            left: 5,
                            right: 5,
                            bottom: 5,
                            height: 18,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            border: '1px dashed',
                            borderColor: getRouteGraphBorderColor(theme),
                            borderRadius: `${theme.shape.borderRadius}px`,
                            color: 'text.secondary',
                            fontSize: 14,
                            lineHeight: 1,
                            opacity: 0,
                            cursor: 'pointer',
                            transition: 'opacity 0.16s ease, border-color 0.16s ease, color 0.16s ease',
                            '&:hover, &:focus-visible': { borderColor: theme.palette.primary.main, color: 'primary.main', outline: 'none', opacity: 1 },
                        })}
                    >
                        +
                    </Box>
                )}
            </Box>
        </NodeTooltip>
    );
};

/** @deprecated Use TierNode */
export const PriorityTierNode = TierNode;

export default TierNode;
