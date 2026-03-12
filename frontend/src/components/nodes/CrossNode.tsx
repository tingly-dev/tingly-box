import { Box, styled, Tooltip, Typography } from '@mui/material';
import React from 'react';

export interface CrossNodeProps {
    size?: number;
    strokeWidth?: number;
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
    strokeWidth = 3,
    active = true,
    label,
    color = 'currentColor',
}) => {
    const halfSize = size / 2;
    const padding = strokeWidth;

    return (
        <Tooltip title={label || 'Union'}>
            <CrossContainer sx={{ width: size, height: size }}>
                <StyledCross active={active}>
                    <svg
                        width={size}
                        height={size}
                        viewBox={`0 0 ${size} ${size}`}
                        fill="none"
                    >
                        {/* Diagonal lines forming an X */}
                        <line
                            x1={padding}
                            y1={padding}
                            x2={size - padding}
                            y2={size - padding}
                            stroke={color}
                            strokeWidth={strokeWidth}
                            strokeLinecap="round"
                        />
                        <line
                            x1={size - padding}
                            y1={padding}
                            x2={padding}
                            y2={size - padding}
                            stroke={color}
                            strokeWidth={strokeWidth}
                            strokeLinecap="round"
                        />
                        {/* Optional: center circle for union emphasis */}
                        <circle
                            cx={halfSize}
                            cy={halfSize}
                            r={strokeWidth * 1.5}
                            fill={color}
                            opacity={active ? 0.3 : 0.1}
                        />
                    </svg>
                </StyledCross>
            </CrossContainer>
        </Tooltip>
    );
};

export default CrossNode;
