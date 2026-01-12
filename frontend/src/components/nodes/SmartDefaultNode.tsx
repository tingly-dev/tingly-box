import {
    Add as AddIcon,
    Add as DefaultIcon,
} from '@mui/icons-material';
import {
    Box,
    Chip,
    IconButton,
    Tooltip,
    Typography,
} from '@mui/material';
import { styled } from '@mui/material/styles';
import React from 'react';

// Node dimensions - smaller for better layout
const NODE_STYLES = {
    width: 220,
    height: 90,
    padding: 8,
} as const;

const { node } = { node: NODE_STYLES };

// SmartDefaultNode Container - styled similar to SmartOpNode but with neutral color
const StyledDefaultNode = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'active',
})<{ active: boolean }>(({ active, theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: node.padding,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: active ? 'warning.main' : 'divider',
    backgroundColor: active ? 'warning.50' : 'background.paper',
    textAlign: 'center',
    width: node.width,
    height: node.height,
    boxShadow: theme.shadows[2],
    transition: 'all 0.2s ease-in-out',
    position: 'relative',
    cursor: 'default',
    opacity: active ? 1 : 0.6,
    '&:hover': {
        borderColor: 'warning.main',
        backgroundColor: 'warning.100',
        boxShadow: theme.shadows[4],
        transform: 'translateY(-2px)',
    }
}));

// Action button container
const ActionButtonsBox = styled(Box)(({ theme }) => ({
    position: 'absolute',
    top: 4,
    right: 4,
    display: 'flex',
    gap: 2,
    opacity: 0,
    transition: 'opacity 0.2s',
    '&:hover': {
        opacity: 1,
    },
}));

const StyledDefaultNodeWrapper = styled(Box)(({ theme }) => ({
    position: 'relative',
    '&:hover .action-buttons': {
        opacity: 1,
    }
}));

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
        <StyledDefaultNodeWrapper>
            <StyledDefaultNode active={active}>
                {/* Content */}
                <Box sx={{ mt: 1, width: '100%' }}>
                    {/* Description */}
                    <Typography
                        variant="body2"
                        sx={{
                            fontWeight: 600,
                            color: 'text.primary',
                            fontSize: '0.85rem',
                            mb: 1,
                        }}
                    >
                        Default Providers
                    </Typography>

                    {/* Summary Info */}
                    <Box
                        sx={{
                            display: 'flex',
                            gap: 1,
                            justifyContent: 'center',
                            alignItems: 'center',
                        }}
                    >
                        <Chip
                            label={`Fallback`}
                            size="small"
                            variant="outlined"
                            sx={{
                                fontSize: '0.7rem',
                                height: 20,
                                borderColor: active ? 'warning.main' : 'divider',
                            }}
                        />
                        <Chip
                            label={`${providersCount} ${providersCount === 1 ? 'Provider' : 'Providers'}`}
                            size="small"
                            variant="outlined"
                            sx={{
                                fontSize: '0.7rem',
                                height: 20,
                                borderColor: active ? 'warning.main' : 'divider',
                            }}
                        />
                    </Box>
                </Box>

                {/* Action Buttons - visible on hover */}
                <ActionButtonsBox className="action-buttons">
                    <Tooltip title="Add provider">
                        <IconButton
                            size="small"
                            onClick={(e) => {
                                e.stopPropagation();
                                onAddProvider();
                            }}
                            disabled={!active}
                            sx={{ p: 0.5, backgroundColor: 'background.paper' }}
                        >
                            <AddIcon sx={{ fontSize: '1rem' }} />
                        </IconButton>
                    </Tooltip>
                </ActionButtonsBox>
            </StyledDefaultNode>
        </StyledDefaultNodeWrapper>
    );
};

export default SmartDefaultNode;
