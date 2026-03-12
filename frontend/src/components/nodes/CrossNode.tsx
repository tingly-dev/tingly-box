import { Box, styled, Tooltip } from '@mui/material';
import CompareArrowsIcon from '@mui/icons-material/CompareArrows';
import React from 'react';

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
        <Tooltip title={label || 'Union'}>
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
        </Tooltip>
    );
};

export default CrossNode;
