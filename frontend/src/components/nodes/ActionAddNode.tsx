import {
    Add as AddIcon,
} from '@/components/icons';
import {
    Box,
    IconButton,
    Typography,
} from '@mui/material';
import { alpha, keyframes, styled } from '@mui/material/styles';
import React from 'react';
import NodeTooltip from './NodeTooltip';
import { getRouteGraphBorderColor, graphNodeBaseHoverStyles, graphNodeHoverStyles, SMALL_NODE_STYLES } from './styles';

const { node } = { node: SMALL_NODE_STYLES };

// Quick Start "Select a Model" fires this so we can point users at the exact
// click target instead of just scrolling near it.
export const SPOTLIGHT_ADD_MODEL_EVENT = 'tb:spotlight-add-model';

const spotlightPulse = keyframes`
    0%   { box-shadow: 0 0 0 0 var(--add-ring); }
    70%  { box-shadow: 0 0 0 10px transparent; }
    100% { box-shadow: 0 0 0 0 transparent; }
`;

// ActionAddNode Container
const StyledAddProviderNode = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'active' && prop !== 'warning' && prop !== 'spotlight',
})<{ active: boolean; warning?: boolean; spotlight?: boolean }>(({ active, warning, spotlight, theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: node.padding,
    borderRadius: theme.shape.borderRadius,
    border: '2px dashed',
    borderColor: getRouteGraphBorderColor(theme),
    backgroundColor: 'background.paper',
    width: node.width,
    height: node.height,
    transition: 'border-color 0.16s ease, background-color 0.16s ease, opacity 0.16s ease, box-shadow 0.18s ease, transform 0.18s ease',
    cursor: active ? 'pointer' : 'default',
    opacity: active ? 1 : 0.5,
    ...graphNodeBaseHoverStyles,
    '&:hover': active ? {
        ...graphNodeHoverStyles(theme),
        borderStyle: 'solid',
    } : {},
    // Spotlight: mirror the hover look and pulse a ring so the node is
    // unmistakable when guidance sends the user here.
    ...(spotlight && active ? {
        borderStyle: 'solid',
        borderColor: theme.palette.primary.main,
        '--add-ring': alpha(theme.palette.primary.main, 0.55),
        animation: `${spotlightPulse} 1.4s ease-out 3`,
    } : {}),
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
    tooltip = 'Add model',
}) => {
    // Temporary pulse when guidance (Quick Start → "Select a Model") asks us to
    // point the user at the add-model target. Auto-clears so it never lingers.
    const [spotlight, setSpotlight] = React.useState(false);
    React.useEffect(() => {
        if (!active) return;
        const onSpotlight = () => {
            setSpotlight(false);
            // restart the animation on a fresh frame even if already pulsing
            requestAnimationFrame(() => setSpotlight(true));
        };
        window.addEventListener(SPOTLIGHT_ADD_MODEL_EVENT, onSpotlight);
        return () => window.removeEventListener(SPOTLIGHT_ADD_MODEL_EVENT, onSpotlight);
    }, [active]);

    React.useEffect(() => {
        if (!spotlight) return;
        const timer = window.setTimeout(() => setSpotlight(false), 4400);
        return () => window.clearTimeout(timer);
    }, [spotlight]);

    return (
        <NodeTooltip title={tooltip} placement="top">
            <StyledAddProviderNode
                active={active}
                warning={warning}
                spotlight={spotlight}
                onClick={active ? onAdd : undefined}
            >
                <AddIcon sx={{ fontSize: 24, color: 'text.secondary' }} />
                <Typography variant="caption" color="text.secondary" textAlign="center" sx={{ fontSize: '0.6rem', lineHeight: 1.1 }}>
                    Add model
                </Typography>
            </StyledAddProviderNode>
        </NodeTooltip>
    );
};

export default ActionAddNode;
