import { Tooltip, TooltipProps } from '@mui/material';
import React from 'react';

// Shared tooltip wrapper for routing-graph nodes.
//
// The routing diagram packs many independent hover targets (provider info,
// priority badge, action buttons, mode toggles). Default MUI delays (100ms
// enter) caused a chain of tooltips to fire as the cursor crossed a node.
// A longer enterDelay means a fast pass-through triggers nothing, and the
// `enterNextDelay` arms the next tooltip briefly so neighbouring targets do
// not stack instantly after one closes.
export const NodeTooltip: React.FC<TooltipProps> = ({
    enterDelay = 600,
    enterNextDelay = 200,
    leaveDelay = 80,
    arrow = true,
    ...rest
}) => (
    <Tooltip
        enterDelay={enterDelay}
        enterNextDelay={enterNextDelay}
        leaveDelay={leaveDelay}
        arrow={arrow}
        {...rest}
    />
);

export default NodeTooltip;
