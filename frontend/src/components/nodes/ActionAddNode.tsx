import {
    Add as AddIcon,
} from '@mui/icons-material';
import {
    Box,
    IconButton,
    Tooltip,
    Typography,
} from '@mui/material';
import { styled } from '@mui/material/styles';
import React from 'react';

// ActionAddNode dimensions
const ADD_PROVIDER_NODE_STYLES = {
    width: 100,
    height: 90,
    padding: 8,
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
    boxShadow: theme.shadows[2],
    transition: 'all 0.2s ease-in-out',
    cursor: active ? 'pointer' : 'default',
    opacity: active ? 1 : 0.5,
    '&:hover': active ? {
        borderColor: warning ? 'warning.main' : 'primary.main',
        backgroundColor: 'action.hover',
        borderStyle: 'solid',
        boxShadow: theme.shadows[4],
        transform: 'translateY(-2px)',
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
        <Tooltip title={tooltip}>
            <StyledAddProviderNode
                active={active}
                warning={warning}
                onClick={active ? onAdd : undefined}
            >
                <AddIcon sx={{ fontSize: 28, color: 'text.secondary' }} />
                <Typography variant="caption" color="text.secondary" textAlign="center" sx={{ fontSize: '0.65rem' }}>
                    Add
                </Typography>
            </StyledAddProviderNode>
        </Tooltip>
    );
};

export default ActionAddNode;
