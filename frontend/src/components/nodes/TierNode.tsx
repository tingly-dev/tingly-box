import { KeyboardArrowDown, KeyboardArrowUp } from '@/components/icons';
import { Box, IconButton, Stack, Typography } from '@mui/material';
import { alpha } from '@mui/material/styles';
import React from 'react';
import { useTranslation } from 'react-i18next';
import NodeTooltip from './NodeTooltip.tsx';
import { getRouteGraphActiveColor, PROVIDER_NODE_STYLES } from './styles.tsx';

export interface TierNodeProps {
    tierIndex: number;
    priority: number;
    active: boolean;
    canMoveUp: boolean;
    canMoveDown: boolean;
    onMoveUp?: () => void;
    onMoveDown?: () => void;
}

export const TIER_NODE_WIDTH = 52;

/** @deprecated Use TIER_NODE_WIDTH */
export const PRIORITY_TIER_NODE_WIDTH = TIER_NODE_WIDTH;

export const TierNode: React.FC<TierNodeProps> = ({
    tierIndex,
    priority,
    active,
    canMoveUp,
    canMoveDown,
    onMoveUp,
    onMoveDown,
}) => {
    const { t } = useTranslation();
    const tierLabel = `T${priority}`;

    return (
        <Box
            sx={(theme) => ({
                position: 'relative',
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
                borderColor: alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.44 : 0.38),
                bgcolor: alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.07 : 0.04),
                opacity: active ? 1 : 0.6,
                transition: 'border-color 0.16s, opacity 0.16s',
                '&:hover .tier-actions': { opacity: 1 },
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
                    {tierLabel}
                </Typography>
            </NodeTooltip>

            <Stack
                className="tier-actions"
                direction="row"
                sx={{
                    position: 'absolute',
                    bottom: 2,
                    left: 0,
                    right: 0,
                    justifyContent: 'center',
                    gap: 0.25,
                    opacity: 0,
                    transition: 'opacity 0.2s',
                }}
            >
                {canMoveUp && (
                    <NodeTooltip title={t('common.moveUp', { defaultValue: 'Move up' })} placement="bottom">
                        <IconButton
                            size="small"
                            onClick={(e) => { e.stopPropagation(); onMoveUp?.(); }}
                            disabled={!active}
                            sx={{ p: 0.25 }}
                        >
                            <KeyboardArrowUp sx={{ fontSize: '0.875rem' }} />
                        </IconButton>
                    </NodeTooltip>
                )}
                {canMoveDown && (
                    <NodeTooltip title={t('common.moveDown', { defaultValue: 'Move down' })} placement="bottom">
                        <IconButton
                            size="small"
                            onClick={(e) => { e.stopPropagation(); onMoveDown?.(); }}
                            disabled={!active}
                            sx={{ p: 0.25 }}
                        >
                            <KeyboardArrowDown sx={{ fontSize: '0.875rem' }} />
                        </IconButton>
                    </NodeTooltip>
                )}
            </Stack>
        </Box>
    );
};

/** @deprecated Use TierNode */
export const PriorityTierNode = TierNode;

export default TierNode;
