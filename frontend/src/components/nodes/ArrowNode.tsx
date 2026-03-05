import { Box, styled } from '@mui/material';
import React from 'react';

export type ArrowDirection = 'forward' | 'back' | 'down' | 'bidirectional';

export interface ArrowNodeProps {
    direction?: ArrowDirection;
    size?: number;
    strokeWidth?: number;
    length?: number;
    arrowHeadSize?: number;
}

const ArrowContainer = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'size',
})<{ size?: number }>(({ size }) => ({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: size || 32,
    height: size || 32,
}));

interface SvgArrowProps {
    strokeWidth: number;
    length: number;
    arrowHeadSize: number;
    color?: string;
}

const ForwardArrow: React.FC<SvgArrowProps> = ({ strokeWidth, length, arrowHeadSize, color = 'currentColor' }) => (
    <svg
        width={length + arrowHeadSize}
        height={strokeWidth * 3}
        viewBox={`0 0 ${length + arrowHeadSize} ${strokeWidth * 3}`}
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
    >
        {/* Shaft */}
        <line
            x1={strokeWidth}
            y1={(strokeWidth * 3) / 2}
            x2={length}
            y2={(strokeWidth * 3) / 2}
            stroke={color}
            strokeWidth={strokeWidth}
            strokeLinecap="round"
        />
        {/* Arrow head */}
        <path
            d={`M ${length - arrowHeadSize} ${(strokeWidth * 3) / 2 - arrowHeadSize} L ${length} ${(strokeWidth * 3) / 2} L ${length - arrowHeadSize} ${(strokeWidth * 3) / 2 + arrowHeadSize}`}
            stroke={color}
            strokeWidth={strokeWidth}
            strokeLinecap="round"
            strokeLinejoin="round"
        />
    </svg>
);

const BackArrow: React.FC<SvgArrowProps> = ({ strokeWidth, length, arrowHeadSize, color = 'currentColor' }) => (
    <svg
        width={length + arrowHeadSize}
        height={strokeWidth * 3}
        viewBox={`0 0 ${length + arrowHeadSize} ${strokeWidth * 3}`}
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
    >
        {/* Shaft */}
        <line
            x1={arrowHeadSize}
            y1={(strokeWidth * 3) / 2}
            x2={length + arrowHeadSize - strokeWidth}
            y2={(strokeWidth * 3) / 2}
            stroke={color}
            strokeWidth={strokeWidth}
            strokeLinecap="round"
        />
        {/* Arrow head */}
        <path
            d={`M ${arrowHeadSize} ${(strokeWidth * 3) / 2} L ${arrowHeadSize + arrowHeadSize} ${(strokeWidth * 3) / 2 - arrowHeadSize} L ${arrowHeadSize + arrowHeadSize} ${(strokeWidth * 3) / 2 + arrowHeadSize}`}
            stroke={color}
            strokeWidth={strokeWidth}
            strokeLinecap="round"
            strokeLinejoin="round"
        />
    </svg>
);

const DownArrow: React.FC<SvgArrowProps> = ({ strokeWidth, length, arrowHeadSize, color = 'currentColor' }) => (
    <svg
        width={strokeWidth * 3}
        height={length + arrowHeadSize}
        viewBox={`0 0 ${strokeWidth * 3} ${length + arrowHeadSize}`}
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
    >
        {/* Shaft */}
        <line
            x1={(strokeWidth * 3) / 2}
            y1={strokeWidth}
            x2={(strokeWidth * 3) / 2}
            y2={length}
            stroke={color}
            strokeWidth={strokeWidth}
            strokeLinecap="round"
        />
        {/* Arrow head */}
        <path
            d={`M ${(strokeWidth * 3) / 2 - arrowHeadSize} ${length - arrowHeadSize} L ${(strokeWidth * 3) / 2} ${length} L ${(strokeWidth * 3) / 2 + arrowHeadSize} ${length - arrowHeadSize}`}
            stroke={color}
            strokeWidth={strokeWidth}
            strokeLinecap="round"
            strokeLinejoin="round"
        />
    </svg>
);

export const ArrowNode: React.FC<ArrowNodeProps> = ({
    direction = 'forward',
    size = 32,
    strokeWidth = 3,
    length = 24,
    arrowHeadSize = 8,
}) => {
    const arrowProps = { strokeWidth, length, arrowHeadSize };

    return (
        <ArrowContainer size={size}>
            {direction === 'bidirectional' ? (
                <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 0.5 }}>
                    <ForwardArrow {...arrowProps} />
                    <BackArrow {...arrowProps} />
                </Box>
            ) : direction === 'down' ? (
                <DownArrow {...arrowProps} />
            ) : direction === 'back' ? (
                <BackArrow {...arrowProps} />
            ) : (
                <ForwardArrow {...arrowProps} />
            )}
        </ArrowContainer>
    );
};
