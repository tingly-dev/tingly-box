import { Box, Typography, styled, Divider, Chip, Tooltip } from '@mui/material';
import { NODE_LAYER_STYLES } from './styles';
import { useCallback } from 'react';

const StyledSmartGuideNode = styled(Box, { shouldForwardProp: (prop) => prop !== 'active' && prop !== 'clickable' })<{
    active: boolean;
    clickable: boolean;
}>(({ active, clickable, theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: 12,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: active ? 'warning.main' : 'divider',
    backgroundColor: active ? 'warning.50' : 'background.paper',
    textAlign: 'center',
    width: 220,
    height: 90,
    boxShadow: theme.shadows[2],
    transition: 'all 0.2s ease-in-out',
    position: 'relative',
    opacity: active ? 1 : 0.6,
    cursor: clickable ? 'pointer' : 'default',
    '&:hover': clickable ? {
        borderColor: 'warning.main',
        boxShadow: theme.shadows[4],
        transform: 'translateY(-2px)',
    } : {},
}));

interface SmartGuideNodeProps {
    provider?: string;
    providerName?: string;  // Display name of the provider
    model?: string;
    active?: boolean;
    onClick?: () => void;
}

const SmartGuideNode: React.FC<SmartGuideNodeProps> = ({
    provider,
    providerName,
    model,
    active = true,
    onClick,
}) => {
    const clickable = !!onClick;
    const hasConfig = !!(provider && model);

    const handleClick = useCallback((event: React.MouseEvent) => {
        event.stopPropagation();
        if (onClick) onClick();
    }, [onClick]);

    return (
        <StyledSmartGuideNode active={active} clickable={clickable} onClick={handleClick}>
            {/* Top Layer - Provider name and model display (same as ProviderNode) */}
            <Box sx={NODE_LAYER_STYLES.topLayer}>
                <Tooltip title={
                    hasConfig
                        ? <>Provider: {providerName || provider}<br/>Model: {model}</>
                        : 'Click to configure SmartGuide model'
                } arrow>
                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 0.5 }}>
                        <Typography
                            variant="body2"
                            color="text.primary"
                            noWrap
                            sx={{
                                ...NODE_LAYER_STYLES.typography,
                                fontStyle: !provider ? 'italic' : 'normal',
                                width: '80px',
                                textAlign: 'center',
                            }}
                        >
                            {providerName || provider || 'select provider'}
                        </Typography>

                        {provider && (
                            <Divider orientation="vertical" flexItem sx={{ mx: 0.5 }} />
                        )}

                        {provider && (
                            <Typography
                                variant="body2"
                                color="text.primary"
                                noWrap
                                sx={{
                                    ...NODE_LAYER_STYLES.typography,
                                    fontStyle: !model ? 'italic' : 'normal',
                                    width: '80px',
                                    textAlign: 'center',
                                }}
                            >
                                {model || 'select model'}
                            </Typography>
                        )}
                    </Box>
                </Tooltip>
            </Box>

            <Divider sx={NODE_LAYER_STYLES.divider} />

            {/* Bottom Layer - Chip showing "SmartGuide" */}
            <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                <Chip
                    label="SmartGuide"
                    size="small"
                    color={hasConfig ? 'warning' : 'default'}
                    sx={{ height: 24, fontSize: '0.7rem', fontWeight: 500 }}
                />
            </Box>
        </StyledSmartGuideNode>
    );
};

export default SmartGuideNode;
