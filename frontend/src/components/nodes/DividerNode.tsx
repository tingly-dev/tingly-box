import { Box, Typography } from '@mui/material';
import { alpha } from '@mui/material/styles';
import React from 'react';
import { getRouteGraphActiveColor, PROVIDER_NODE_STYLES } from './styles.tsx';

export interface DividerNodeProps {
    /** Label displayed at the center of the divider, e.g. "Priority 2" or "均衡" */
    label: string;
    active?: boolean;
}

/**
 * A vertical divider node placed between priority-tier groups in the service list.
 * Renders a thin colored line with a text label centered on it.
 */
export const DividerNode: React.FC<DividerNodeProps> = ({ label, active = true }) => {
    return (
        <Box
            sx={{
                position: 'relative',
                width: 40,
                height: PROVIDER_NODE_STYLES.height,
                flexShrink: 0,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                opacity: active ? 1 : 0.5,
            }}
        >
            {/* Vertical line */}
            <Box
                sx={(theme) => ({
                    position: 'absolute',
                    left: '50%',
                    top: 6,
                    bottom: 6,
                    width: '1px',
                    backgroundColor: alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.35 : 0.28),
                    borderRadius: '1px',
                })}
            />
            {/* Label chip centered on the line */}
            <Box
                sx={(theme) => ({
                    position: 'relative',
                    zIndex: 1,
                    px: 0.75,
                    py: 0.25,
                    borderRadius: 0.75,
                    border: '1px solid',
                    borderColor: alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.35 : 0.25),
                    backgroundColor: 'background.paper',
                    lineHeight: 1,
                })}
            >
                <Typography
                    variant="caption"
                    sx={(theme) => ({
                        fontSize: '0.6rem',
                        fontWeight: 700,
                        color: alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.75 : 0.65),
                        whiteSpace: 'nowrap',
                        letterSpacing: '0.02em',
                    })}
                >
                    {label}
                </Typography>
            </Box>
        </Box>
    );
};

export default DividerNode;
