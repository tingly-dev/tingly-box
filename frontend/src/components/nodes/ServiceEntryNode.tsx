import { Box, Typography } from '@mui/material';
import { alpha } from '@mui/material/styles';
import React from 'react';
import { useTranslation } from 'react-i18next';
import NodeTooltip from './NodeTooltip';
import {
    getRouteGraphActiveColor,
    NODE_LAYER_STYLES,
    SMART_NODE_STYLES,
    StyledSmartNodeWrapper,
} from './styles.tsx';
import { fontSizes } from '@/theme/fonts';

export interface ServiceEntryNodeProps {
    providersCount: number;
    active: boolean;
    /** Hover teaching for the node. */
    tooltip?: React.ReactNode;
    /** Conditions are off: Default is the whole rule and stands as tall as a
     * service node; on hover its label shifts up and a dashed "+ label" appears
     * inside it (same idea as the last tier node's +). */
    onAddCondition?: () => void;
    addConditionLabel?: string;
    /** Tooltip of the inline add button (the smart-routing guide). */
    addTooltip?: React.ReactNode;
}

export const ServiceEntryNode: React.FC<ServiceEntryNodeProps> = ({
    active,
    tooltip,
    onAddCondition,
    addConditionLabel,
    addTooltip,
}) => {
    const { t } = useTranslation();
    const node = (
        <StyledSmartNodeWrapper>
            <Box
                sx={(theme) => ({
                    position: 'relative',
                    width: SMART_NODE_STYLES.width,
                    height: onAddCondition ? SMART_NODE_STYLES.height : 34,
                    boxSizing: 'border-box',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    borderRadius: `${theme.shape.borderRadius}px`,
                    border: '1px solid',
                    borderColor: alpha(
                        getRouteGraphActiveColor(theme),
                        theme.palette.mode === 'dark' ? 0.45 : 0.38,
                    ),
                    opacity: active ? 1 : 0.6,
                    transition: 'border-color 0.16s ease, opacity 0.16s ease',
                    ...(onAddCondition && {
                        '&:hover .default-node-add': { opacity: 1 },
                        '&:hover .default-node-label': { transform: 'translateY(-10px)' },
                    }),
                })}
            >
                <NodeTooltip title={onAddCondition ? tooltip ?? '' : ''} placement="top" disableHoverListener={!onAddCondition || !tooltip}>
                    <Typography
                        className="default-node-label"
                        sx={{ ...NODE_LAYER_STYLES.typography, color: 'text.secondary', transition: 'transform 0.16s ease' }}
                    >
                        {t('rule.ruleConditions.defaultTitle')}
                    </Typography>
                </NodeTooltip>
                {onAddCondition && (
                    <NodeTooltip title={addTooltip ?? ''} placement="top" disableHoverListener={!addTooltip}>
                    <Box
                        className="default-node-add"
                        role="button"
                        tabIndex={active ? 0 : -1}
                        aria-label={addConditionLabel}
                        onClick={active ? (e) => { e.stopPropagation(); onAddCondition(); } : undefined}
                        onKeyDown={(e) => { if (active && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); onAddCondition(); } }}
                        sx={(theme) => ({
                            position: 'absolute',
                            left: 6,
                            right: 6,
                            bottom: 6,
                            height: 22,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            border: '1px dashed',
                            borderColor: theme.palette.divider,
                            borderRadius: `${theme.shape.borderRadius}px`,
                            color: 'text.secondary',
                            fontSize: fontSizes.xs,
                            opacity: 0,
                            cursor: active ? 'pointer' : 'default',
                            transition: 'opacity 0.16s ease, border-color 0.16s ease, color 0.16s ease',
                            '&:hover, &:focus-visible': { borderColor: theme.palette.primary.main, color: 'primary.main', outline: 'none', opacity: 1 },
                        })}
                    >
                        + {addConditionLabel}
                    </Box>
                    </NodeTooltip>
                )}
            </Box>
        </StyledSmartNodeWrapper>
    );
    return tooltip && !onAddCondition ? <NodeTooltip title={tooltip} placement="top">{node}</NodeTooltip> : node;
};

export default ServiceEntryNode;
