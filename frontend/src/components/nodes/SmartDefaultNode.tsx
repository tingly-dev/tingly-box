import {
    Box,
    Button,
    Typography,
} from '@mui/material';
import { alpha } from '@mui/material/styles';
import React from 'react';
import {
    getRouteGraphActiveColor,
    NODE_LAYER_STYLES,
    SMART_NODE_STYLES,
    StyledSmartNodeWrapper,
} from './styles.tsx';

export interface DefaultNodeProps {
    providersCount: number;
    active: boolean;
    onAddProvider: () => void;
}

export const SmartDefaultNode: React.FC<DefaultNodeProps> = ({
    providersCount,
    active,
    onAddProvider,
}) => {
    return (
        <StyledSmartNodeWrapper>
            <Button
                variant="outlined"
                disabled={!active}
                onClick={active ? onAddProvider : undefined}
                sx={(theme) => ({
                    width: SMART_NODE_STYLES.width,
                    height: 36,
                    borderColor: alpha(
                        getRouteGraphActiveColor(theme),
                        theme.palette.mode === 'dark' ? 0.72 : 0.82
                    ),
                    color: getRouteGraphActiveColor(theme),
                    backgroundColor: 'transparent',
                    opacity: active ? 1 : 0.6,
                    transition: 'border-color 0.16s ease, background-color 0.16s ease, opacity 0.16s ease, box-shadow 0.18s ease',
                    '&:hover': active ? {
                        borderColor: getRouteGraphActiveColor(theme),
                        backgroundColor: alpha(
                            getRouteGraphActiveColor(theme),
                            theme.palette.mode === 'dark' ? 0.12 : 0.06
                        ),
                        boxShadow: `0 0 0 3px ${alpha(
                            getRouteGraphActiveColor(theme),
                            theme.palette.mode === 'dark' ? 0.14 : 0.10
                        )}`,
                    } : {},
                    '&.Mui-disabled': {
                        borderColor: theme.palette.divider,
                        color: theme.palette.text.disabled,
                        opacity: 0.6,
                    },
                })}
            >
                <Typography variant="body2" sx={NODE_LAYER_STYLES.typography}>
                    Default
                </Typography>
            </Button>
        </StyledSmartNodeWrapper>
    );
};

export default SmartDefaultNode;
