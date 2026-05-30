import { Box, Typography } from '@mui/material';
import { alpha } from '@mui/material/styles';
import React from 'react';
import {
    getRouteGraphActiveColor,
    NODE_LAYER_STYLES,
    SMART_NODE_STYLES,
    StyledSmartNodeWrapper,
} from './styles.tsx';

export interface ServiceEntryNodeProps {
    providersCount: number;
    active: boolean;
}

export const ServiceEntryNode: React.FC<ServiceEntryNodeProps> = ({ providersCount, active }) => {
    return (
        <StyledSmartNodeWrapper>
            <Box
                sx={(theme) => ({
                    width: SMART_NODE_STYLES.width,
                    height: 36,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    borderRadius: `${theme.shape.borderRadius}px`,
                    border: '1px solid',
                    borderColor: alpha(
                        getRouteGraphActiveColor(theme),
                        theme.palette.mode === 'dark' ? 0.45 : 0.38,
                    ),
                    opacity: active ? 1 : 0.6,
                    transition: 'border-color 0.16s ease, opacity 0.16s ease',
                })}
            >
                <Typography sx={{ ...NODE_LAYER_STYLES.typography, color: 'text.secondary' }}>
                    Default
                </Typography>
            </Box>
        </StyledSmartNodeWrapper>
    );
};

export default ServiceEntryNode;
