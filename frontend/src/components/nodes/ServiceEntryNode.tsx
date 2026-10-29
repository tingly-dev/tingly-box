import { Box, Typography } from '@mui/material';
import { alpha } from '@mui/material/styles';
import React from 'react';
import { useTranslation } from 'react-i18next';
import NodeTooltip from './NodeTooltip';
import {
    getRouteGraphActiveColor,
    NODE_LAYER_STYLES,
    SMALL_NODE_STYLES,
    SMART_NODE_STYLES,
    StyledSmartNodeWrapper,
} from './styles.tsx';

export interface ServiceEntryNodeProps {
    providersCount: number;
    active: boolean;
    /** Sits next to an Add condition node: a service node's height, narrower. */
    compact?: boolean;
    /** Hover teaching for the node. */
    tooltip?: React.ReactNode;
}

export const ServiceEntryNode: React.FC<ServiceEntryNodeProps> = ({ active, compact, tooltip }) => {
    const { t } = useTranslation();
    const node = (
        <StyledSmartNodeWrapper>
            <Box
                sx={(theme) => ({
                    width: compact ? SMART_NODE_STYLES.width - SMALL_NODE_STYLES.width - 8 : SMART_NODE_STYLES.width,
                    height: compact ? SMART_NODE_STYLES.height : 34,
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
                })}
            >
                <Typography sx={{ ...NODE_LAYER_STYLES.typography, color: 'text.secondary' }}>
                    {t('rule.ruleConditions.defaultTitle')}
                </Typography>
            </Box>
        </StyledSmartNodeWrapper>
    );
    return tooltip ? <NodeTooltip title={tooltip} placement="top">{node}</NodeTooltip> : node;
};

export default ServiceEntryNode;
