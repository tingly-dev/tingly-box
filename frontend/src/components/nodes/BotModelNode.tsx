import { Box, Typography, styled, Divider, Chip, Tooltip } from '@mui/material';
import { Warning as WarningIcon } from '@mui/icons-material';
import { NODE_LAYER_STYLES } from './styles';
import { useCallback } from 'react';

const StyledBotModelNode = styled(Box, { shouldForwardProp: (prop) => prop !== 'active' && prop !== 'clickable' && prop !== 'hasConfig' })<{
    active: boolean;
    clickable: boolean;
    hasConfig: boolean;
}>(({ active, clickable, hasConfig, theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: 12,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: hasConfig ? (active ? 'warning.main' : 'divider') : 'warning.main',
    backgroundColor: hasConfig ? (active ? 'warning.50' : 'background.paper') : 'warning.50',
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

interface BotModelNodeProps {
    provider?: string;
    providerName?: string;  // Display name of the provider
    model?: string;
    active?: boolean;
    onClick?: () => void;
}

const BotModelNode: React.FC<BotModelNodeProps> = ({
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
        <StyledBotModelNode active={active} clickable={clickable} hasConfig={hasConfig} onClick={handleClick}>
            {/* Top Layer - Provider name and model display (same as ProviderNode) */}
            <Box sx={NODE_LAYER_STYLES.topLayer}>
                <Tooltip title={
                    hasConfig
                        ? <>Provider: {providerName || provider}<br/>Model: {model}</>
                        : 'Click to configure bot model'
                } arrow>
                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 0.5 }}>
                        {/* Warning icon when model not configured - inline with text */}
                        {active && !hasConfig && (
                            <WarningIcon
                                sx={{
                                    fontSize: '1rem',
                                    color: 'warning.main',
                                }}
                            />
                        )}

                        <Typography
                            variant="body2"
                            color="text.primary"
                            noWrap
                            sx={{
                                ...NODE_LAYER_STYLES.typography,
                                fontStyle: !provider ? 'italic' : 'normal',
                                width: '100px',
                                textAlign: 'center',
                            }}
                        >
                            {providerName || provider || 'select model'}
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
                                    width: '70px',
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

            {/* Bottom Layer - Chip showing bot model */}
            <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                <Chip
                    label="Model"
                    size="small"
                    color={hasConfig ? 'warning' : 'default'}
                    sx={{ height: 24, fontSize: '0.7rem', fontWeight: 500 }}
                />
            </Box>
        </StyledBotModelNode>
    );
};

export default BotModelNode;
