import { Box, styled } from '@mui/material';
import { alpha } from '@mui/material/styles';
import React from 'react';

export type ArrowDirection = 'forward' | 'back' | 'down' | 'bidirectional';

export interface ArrowNodeProps {
    direction?: ArrowDirection;
    size?: number;
    strokeWidth?: number;
    length?: number;
    arrowHeadSize?: number;
}

const ArrowContainer = styled(Box)(({ theme }) => ({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: alpha(theme.palette.text.secondary, 0.7),
}));

interface SvgArrowProps {
    strokeWidth: number;
    length: number;
    arrowHeadSize: number;
    color?: string;
}

const ForwardArrow: React.FC<SvgArrowProps> = ({ strokeWidth, length, arrowHeadSize, color = 'currentColor' }) => {
    const svgWidth = length + arrowHeadSize;
    const svgHeight = arrowHeadSize * 2 + strokeWidth * 2;
    const centerY = svgHeight / 2;
    const tipX = svgWidth - strokeWidth;
    const half = strokeWidth / 2;

    return (
        <svg width={svgWidth} height={svgHeight} viewBox={`0 0 ${svgWidth} ${svgHeight}`} fill="none">
            <line
                x1={half} y1={centerY}
                x2={tipX - half} y2={centerY}
                stroke={color} strokeWidth={strokeWidth} strokeLinecap="round"
            />
            <path
                d={`M ${tipX - arrowHeadSize} ${centerY - arrowHeadSize} L ${tipX} ${centerY} L ${tipX - arrowHeadSize} ${centerY + arrowHeadSize}`}
                stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" fill="none"
            />
        </svg>
    );
};

const BackArrow: React.FC<SvgArrowProps> = ({ strokeWidth, length, arrowHeadSize, color = 'currentColor' }) => {
    const svgWidth = length + arrowHeadSize;
    const svgHeight = arrowHeadSize * 2 + strokeWidth * 2;
    const centerY = svgHeight / 2;
    const tipX = strokeWidth;
    const half = strokeWidth / 2;

    return (
        <svg width={svgWidth} height={svgHeight} viewBox={`0 0 ${svgWidth} ${svgHeight}`} fill="none">
            <line
                x1={tipX + half} y1={centerY}
                x2={svgWidth - half} y2={centerY}
                stroke={color} strokeWidth={strokeWidth} strokeLinecap="round"
            />
            <path
                d={`M ${tipX + arrowHeadSize} ${centerY - arrowHeadSize} L ${tipX} ${centerY} L ${tipX + arrowHeadSize} ${centerY + arrowHeadSize}`}
                stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" fill="none"
            />
        </svg>
    );
};

const DownArrow: React.FC<SvgArrowProps> = ({ strokeWidth, length, arrowHeadSize, color = 'currentColor' }) => {
    const svgWidth = arrowHeadSize * 2 + strokeWidth * 2;
    const svgHeight = length + arrowHeadSize;
    const centerX = svgWidth / 2;
    const tipY = svgHeight - strokeWidth;
    const half = strokeWidth / 2;

    return (
        <svg width={svgWidth} height={svgHeight} viewBox={`0 0 ${svgWidth} ${svgHeight}`} fill="none">
            <line
                x1={centerX} y1={half}
                x2={centerX} y2={tipY - half}
                stroke={color} strokeWidth={strokeWidth} strokeLinecap="round"
            />
            <path
                d={`M ${centerX - arrowHeadSize} ${tipY - arrowHeadSize} L ${centerX} ${tipY} L ${centerX + arrowHeadSize} ${tipY - arrowHeadSize}`}
                stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" fill="none"
            />
        </svg>
    );
};

export const ArrowNode: React.FC<ArrowNodeProps> = ({
    direction = 'forward',
    size = 32,
    strokeWidth = 1.75,
    length = 30,
    arrowHeadSize = 5,
}) => {
    const arrowProps = { strokeWidth, length, arrowHeadSize };

    return (
        <ArrowContainer sx={{ width: size, height: size }}>
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
