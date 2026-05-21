import {
    Add as AddIcon,
} from '@mui/icons-material';
import {
    Box,
    IconButton,
    Typography,
} from '@mui/material';
import { styled } from '@mui/material/styles';
import React from 'react';
import NodeTooltip from './NodeTooltip';
import { graphNodeBaseHoverStyles, graphNodeHoverStyles } from './styles';

// ActionAddNode dimensions
const ADD_PROVIDER_NODE_STYLES = {
    width: 100,
    height: 72,
    padding: 5,
} as const;

const { node } = { node: ADD_PROVIDER_NODE_STYLES };

// ActionAddNode Container
const StyledAddProviderNode = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'active' && prop !== 'warning',
})<{ active: boolean; warning?: boolean }>(({ active, warning, theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: node.padding,
    borderRadius: theme.shape.borderRadius,
    border: 'dashed',
    borderColor: 'divider',
    backgroundColor: 'background.paper',
    width: node.width,
    height: node.height,
    boxShadow: 'none',
    transition: 'border-color 0.16s ease, background-color 0.16s ease, opacity 0.16s ease, box-shadow 0.18s ease, transform 0.18s ease',
    cursor: active ? 'pointer' : 'default',
    opacity: active ? 1 : 0.5,
    ...graphNodeBaseHoverStyles,
    '&:hover': active ? {
        ...graphNodeHoverStyles(theme),
        borderStyle: 'solid',
    } : {},
}));

export interface AddProviderNodeProps {
    active: boolean;
    warning?: boolean;
    onAdd: () => void;
    tooltip?: string;
}

export const ActionAddNode: React.FC<AddProviderNodeProps> = ({
    active,
    warning = false,
    onAdd,
    tooltip = 'Add provider',
}) => {
    return (
        <NodeTooltip title={tooltip} placement="top">
            <StyledAddProviderNode
                active={active}
                warning={warning}
                onClick={active ? onAdd : undefined}
            >
                <AddIcon sx={{ fontSize: 24, color: 'text.secondary' }} />
                <Typography variant="caption" color="text.secondary" textAlign="center" sx={{ fontSize: '0.6rem', lineHeight: 1.1 }}>
                    Add
                </Typography>
            </StyledAddProviderNode>
        </NodeTooltip>
    );
};

export default ActionAddNode;
