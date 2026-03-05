import { ArrowBack as ArrowBackIcon, ArrowDownward as ArrowDownIcon, ArrowForward as ArrowForwardIcon } from '@mui/icons-material';
import { Box, styled } from '@mui/material';
import React from 'react';

export type ArrowDirection = 'forward' | 'back' | 'down' | 'bidirectional';

export interface ArrowNodeProps {
    direction?: ArrowDirection;
    size?: number;
    strokeWidth?: number;
}

const ArrowContainer = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'strokeWidth' && prop !== 'size',
})<{ size?: number; strokeWidth?: number }>(({ size, strokeWidth }) => ({
    display: 'flex',
    alignItems: 'center',
    color: 'text.secondary',
    '& svg': {
        fontSize: size || '2rem',
        strokeWidth: strokeWidth || 3,
    }
}));

export const ArrowNode: React.FC<ArrowNodeProps> = ({
    direction = 'forward',
    size = 32,
    strokeWidth = 4,
}) => {
    return (
        <ArrowContainer size={size} strokeWidth={strokeWidth}>
            {direction === 'bidirectional' ? (
                <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 2 }}>
                    <ArrowForwardIcon sx={{ transform: 'rotate(45deg)' }} />
                    <ArrowBackIcon sx={{ transform: 'rotate(-45deg)' }} />
                </Box>
            ) : direction === 'down' ? (
                <ArrowDownIcon />
            ) : direction === 'back' ? (
                <ArrowBackIcon />
            ) : (
                <ArrowForwardIcon width={100} />
            )}
        </ArrowContainer>
    );
};
