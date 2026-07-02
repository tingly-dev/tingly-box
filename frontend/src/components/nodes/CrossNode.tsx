import { Box, styled } from '@mui/material';
import { CompareArrows as CompareArrowsIcon } from '@/components/icons';
import React from 'react';
import NodeTooltip from './NodeTooltip';

export interface CrossNodeProps {
    size?: number;
    active?: boolean;
    label?: string;
    color?: string;
}

const CrossContainer = styled(Box)(({ theme }) => ({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: theme.palette.text.secondary,
}));

const StyledCross = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'active',
})<{ active: boolean }>(({ active, theme }) => ({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    opacity: active ? 1 : 0.4,
    transition: 'opacity 0.2s ease-in-out',
}));

const CrossNode: React.FC<CrossNodeProps> = ({
    size = 32,
    active = true,
    label,
    color = 'currentColor',
}) => {
    return (
        <NodeTooltip title={label || 'Union'} placement="top">
            <CrossContainer sx={{ width: size, height: size }}>
                <StyledCross active={active}>
                    <CompareArrowsIcon
                        sx={{
                            fontSize: size,
                            color,
                        }}
                    />
                </StyledCross>
            </CrossContainer>
        </NodeTooltip>
    );
};

export default CrossNode;
