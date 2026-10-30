import { Box, Typography } from '@mui/material';
import { alpha } from '@mui/material/styles';
import React from 'react';
import { useTranslation } from 'react-i18next';
import ActionAddNode from './ActionAddNode';
import NodeTooltip from './NodeTooltip';
import {
    getRouteGraphActiveColor,
    NODE_LAYER_STYLES,
    PROVIDER_NODE_STYLES,
    SMART_NODE_STYLES,
    StyledSmartNodeWrapper,
} from './styles.tsx';

/** As tall as a "wide" ActionAddNode, so the entry and the add node below it match. */
const ENTRY_NODE_HEIGHT = 34;

export interface ServiceEntryNodeProps {
    providersCount: number;
    active: boolean;
    /** Hover teaching for the node. */
    tooltip?: React.ReactNode;
    /** Conditions are off: the node is the whole rule's entry ("Request") and
     * "+ Add smart condition" sits below it. Conditions on: it is the path for
     * requests no condition matched ("Others"), with nothing below it. */
    onAddCondition?: () => void;
    addConditionLabel?: string;
    /** Tooltip of the add node (the smart-routing guide). */
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
    // The node lines up with the arrow and T0, which sit in the middle of a
    // service-node-tall row; the add node hangs below it.
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
                    {onAddCondition ? t('rule.ruleConditions.requestTitle') : t('rule.ruleConditions.othersTitle')}
                </Typography>
            </Box>
        </StyledSmartNodeWrapper>
    );
    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1, pt: `${top}px` }}>
            {tooltip ? <NodeTooltip title={tooltip} placement="top">{node}</NodeTooltip> : node}
            {onAddCondition && (
                <ActionAddNode
                    label={addConditionLabel}
                    active={active}
                    onAdd={onAddCondition}
                    tooltip={addTooltip}
                />
            )}
        </Box>
    );
};

export default ServiceEntryNode;
