import { Box, styled } from '@mui/material';
import React from 'react';

export type ArrowDirection = 'forward' | 'back' | 'down' | 'bidirectional';

export interface ArrowNodeProps {
    direction?: ArrowDirection;
    size?: number;
    strokeWidth?: number;
    length?: number;
    arrowHeadSize?: number;
    flowing?: boolean;
    flowSpeed?: number;
}

const ArrowContainer = styled(Box)(({ theme }) => ({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: theme.palette.text.secondary,
}));

const FlowingLine = styled('line')<{
    flowing: boolean;
    flowSpeed: number;
}>(({ flowing, flowSpeed }) => ({
    ...(flowing && {
        strokeDasharray: '6 4',
        animation: `flow ${1 / flowSpeed}s linear infinite`,
        '@keyframes flow': {
            '0%': { strokeDashoffset: '10' },
            '100%': { strokeDashoffset: '0' },
        },
    }),
}));

interface SvgArrowProps {
    strokeWidth: number;
    length: number;
    arrowHeadSize: number;
    color?: string;
    flowing?: boolean;
    flowSpeed?: number;
}

const ForwardArrow: React.FC<SvgArrowProps> = ({
    strokeWidth,
    length,
    arrowHeadSize,
    color = 'currentColor',
    flowing = false,
    flowSpeed = 1
}) => {
    const svgWidth = length + arrowHeadSize;
    const svgHeight = strokeWidth * 4;
    const centerY = svgHeight / 2;
    // Extend line into arrow head - goes almost to the tip
    const lineEndX = svgWidth - strokeWidth;

    return (
        <svg
            width={svgWidth}
            height={svgHeight}
            viewBox={`0 0 ${svgWidth} ${svgHeight}`}
            fill="none"
        >
            {/* Shaft - extends into arrow head */}
            <FlowingLine
                flowing={flowing}
                flowSpeed={flowSpeed}
                x1={strokeWidth * 1.5}
                y1={centerY}
                x2={lineEndX}
                y2={centerY}
                stroke={color}
                strokeWidth={strokeWidth}
                strokeLinecap="round"
            />
            {/* Arrow head */}
            <path
                d={`M ${lineEndX - arrowHeadSize} ${centerY - arrowHeadSize} L ${svgWidth - strokeWidth} ${centerY} L ${lineEndX - arrowHeadSize} ${centerY + arrowHeadSize}`}
                stroke={color}
                strokeWidth={strokeWidth}
                strokeLinecap="round"
                strokeLinejoin="round"
                fill="none"
            />
        </svg>
    );
};

const BackArrow: React.FC<SvgArrowProps> = ({
    strokeWidth,
    length,
    arrowHeadSize,
    color = 'currentColor',
    flowing = false,
    flowSpeed = 1
}) => {
    const svgWidth = length + arrowHeadSize;
    const svgHeight = strokeWidth * 4;
    const centerY = svgHeight / 2;
    // Extend line into arrow head - goes almost to the tip
    const lineStartX = strokeWidth;

    return (
        <svg
            width={svgWidth}
            height={svgHeight}
            viewBox={`0 0 ${svgWidth} ${svgHeight}`}
            fill="none"
        >
            {/* Shaft - extends into arrow head */}
            <FlowingLine
                flowing={flowing}
                flowSpeed={flowSpeed}
                x1={lineStartX}
                y1={centerY}
                x2={svgWidth - strokeWidth}
                y2={centerY}
                stroke={color}
                strokeWidth={strokeWidth}
                strokeLinecap="round"
                style={{ animationDirection: 'reverse' }}
            />
            {/* Arrow head */}
            <path
                d={`M ${lineStartX + arrowHeadSize} ${centerY - arrowHeadSize} L ${lineStartX} ${centerY} L ${lineStartX + arrowHeadSize} ${centerY + arrowHeadSize}`}
                stroke={color}
                strokeWidth={strokeWidth}
                strokeLinecap="round"
                strokeLinejoin="round"
                fill="none"
            />
        </svg>
    );
};

const DownArrow: React.FC<SvgArrowProps> = ({
    strokeWidth,
    length,
    arrowHeadSize,
    color = 'currentColor',
    flowing = false,
    flowSpeed = 1
}) => {
    const svgWidth = strokeWidth * 4;
    const svgHeight = length + arrowHeadSize;
    const centerX = svgWidth / 2;
    // Extend line into arrow head - goes almost to the tip
    const lineEndY = svgHeight - strokeWidth;

    return (
        <svg
            width={svgWidth}
            height={svgHeight}
            viewBox={`0 0 ${svgWidth} ${svgHeight}`}
            fill="none"
        >
            {/* Shaft - extends into arrow head */}
            <FlowingLine
                flowing={flowing}
                flowSpeed={flowSpeed}
                x1={centerX}
                y1={strokeWidth * 1.5}
                x2={centerX}
                y2={lineEndY}
                stroke={color}
                strokeWidth={strokeWidth}
                strokeLinecap="round"
            />
            {/* Arrow head */}
            <path
                d={`M ${centerX - arrowHeadSize} ${lineEndY - arrowHeadSize} L ${centerX} ${svgHeight - strokeWidth} L ${centerX + arrowHeadSize} ${lineEndY - arrowHeadSize}`}
                stroke={color}
                strokeWidth={strokeWidth}
                strokeLinecap="round"
                strokeLinejoin="round"
                fill="none"
            />
        </svg>
    );
};

export const ArrowNode: React.FC<ArrowNodeProps> = ({
    direction = 'forward',
    size = 32,
    strokeWidth = 3,
    length = 32,
    arrowHeadSize = 8,
    flowing = false,
    flowSpeed = 1,
}) => {
    const arrowProps = { strokeWidth, length, arrowHeadSize, flowing, flowSpeed };

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
