import { Box, Typography } from '@mui/material';
import { alpha } from '@mui/material/styles';
import React from 'react';
import { useTranslation } from 'react-i18next';
import NodeTooltip from './NodeTooltip';
import {
    getRouteGraphActiveColor,
    NODE_LAYER_STYLES,
    PROVIDER_NODE_STYLES,
    SMART_NODE_STYLES,
    StyledSmartNodeWrapper,
} from './styles.tsx';

/** As tall as a "wide" ActionAddNode, like the add / turn-off controls. */
const ENTRY_NODE_HEIGHT = 34;

export interface ServiceEntryNodeProps {
    providersCount: number;
    active: boolean;
    /** Hover teaching for the node. */
    tooltip?: React.ReactNode;
    /** Conditions on: the node is the path for requests no condition matched
     * ("Others"). Off: it is the whole rule's entry ("Request"). */
    others?: boolean;
}

export const ServiceEntryNode: React.FC<ServiceEntryNodeProps> = ({
    active,
    tooltip,
    others,
}) => {
    const { t } = useTranslation();
    // The node lines up with the arrow and T0, which sit in the middle of a
    // service-node-tall row.
    const top = (PROVIDER_NODE_STYLES.height - ENTRY_NODE_HEIGHT) / 2;
    const node = (
        <StyledSmartNodeWrapper>
            <Box
                sx={(theme) => ({
                    width: SMART_NODE_STYLES.width,
                    height: ENTRY_NODE_HEIGHT,
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
                })}
            >
                <Typography sx={{ ...NODE_LAYER_STYLES.typography, color: 'text.secondary' }}>
                    {others ? t('rule.ruleConditions.othersTitle') : t('rule.ruleConditions.requestTitle')}
                </Typography>
            </Box>
        </StyledSmartNodeWrapper>
    );
    return (
        <Box sx={{ pt: `${top}px` }}>
            {tooltip ? <NodeTooltip title={tooltip} placement="top">{node}</NodeTooltip> : node}
        </Box>
    );
};

export default ServiceEntryNode;
