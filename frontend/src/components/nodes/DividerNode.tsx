import { Box } from '@mui/material';
import { alpha } from '@mui/material/styles';
import React from 'react';
import { getRouteGraphActiveColor, PROVIDER_NODE_STYLES } from './styles.tsx';

export interface DividerNodeProps {
    active?: boolean;
}

/**
 * A vertical divider placed between priority-tier groups in the service list.
 */
export const DividerNode: React.FC<DividerNodeProps> = ({ active = true }) => {
    return (
        <Box
            sx={{
                width: 16,
                height: PROVIDER_NODE_STYLES.height,
                flexShrink: 0,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                opacity: active ? 1 : 0.5,
            }}
        >
            <Box
                sx={(theme) => ({
                    width: '1px',
                    height: '60%',
                    backgroundColor: alpha(
                        getRouteGraphActiveColor(theme),
                        theme.palette.mode === 'dark' ? 0.30 : 0.22,
                    ),
                    borderRadius: '1px',
                })}
            />
        </Box>
    );
};

export default DividerNode;
