import {
    Add as AddIcon,
    Add as DefaultIcon,
} from '@mui/icons-material';
import {
    Box,
    IconButton,
    Tooltip,
    Typography,
} from '@mui/material';
import React from 'react';
import {
    ActionButtonsBox,
    StyledSmartNodeWarning,
    StyledSmartNodeWrapper,
} from './styles.tsx';

export interface DefaultNodeProps {
    providersCount: number;
    active: boolean;
    onAddProvider: () => void;
}

export const SmartFallbackNode: React.FC<DefaultNodeProps> = ({
    providersCount,
    active,
    onAddProvider,
}) => {
    return (
        <StyledSmartNodeWrapper>
            <StyledSmartNodeWarning active={active}>
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
                        Fallback
                    </Typography>

                    {/* Summary Info */}
                    <Box
                        sx={{
                            width: '100%',
                        }}
                    >
                        <Box
                            sx={{
                                width: '100%',
                                p: 1,
                                border: '1px solid',
                                borderColor: active ? 'warning.main' : 'divider',
                                borderRadius: 1,
                                backgroundColor: 'background.paper',
                                transition: 'all 0.2s',
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                            }}
                        >
                            <Typography
                                variant="body2"
                                sx={{
                                    fontSize: '0.8rem',
                                    color: active ? 'warning.main' : 'text.secondary',
                                    fontWeight: 500,
                                }}
                            >
                                Fallback
                            </Typography>
                        </Box>
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
            </StyledSmartNodeWarning>
        </StyledSmartNodeWrapper>
    );
};

export default SmartFallbackNode;
