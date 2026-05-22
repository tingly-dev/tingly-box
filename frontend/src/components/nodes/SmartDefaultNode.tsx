import {
    Box,
    Typography,
} from '@mui/material';
import { alpha } from '@mui/material/styles';
import React from 'react';
import {
    getRouteGraphActiveColor,
    StyledSmartNodeWarning,
    StyledSmartNodeWrapper,
} from './styles.tsx';

const DEFAULT_NODE_INTERNAL_STYLES = {
    contentHeight: 62,
    fieldHeight: 31,
} as const;

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
            <StyledSmartNodeWarning active={active}>
                {/* Content */}
                <Box
                    sx={{
                        width: '100%',
                        height: DEFAULT_NODE_INTERNAL_STYLES.contentHeight,
                        display: 'flex',
                        alignItems: 'center',
                    }}
                >
                    {/* Summary Info */}
                    <Box
                        sx={{
                            width: '100%',
                        }}
                    >
                        <Box
                            sx={(theme) => ({
                                width: '100%',
                                height: DEFAULT_NODE_INTERNAL_STYLES.fieldHeight,
                                px: 1,
                                border: '1px solid',
                                borderColor: alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.34 : 0.22),
                                borderRadius: 1,
                                backgroundColor: 'background.paper',
                                transition: 'border-color 0.16s ease, background-color 0.16s ease',
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                            })}
                        >
                            <Typography
                                variant="body2"
                                sx={{
                                    fontSize: '0.8rem',
                                    color: 'text.secondary',
                                    fontWeight: 500,
                                    lineHeight: 1,
                                }}
                            >
                                Default
                            </Typography>
                        </Box>
                    </Box>
                </Box>

            </StyledSmartNodeWarning>
        </StyledSmartNodeWrapper>
    );
};

export default SmartDefaultNode;
