import { Box, Typography, Divider } from '@mui/material';
import { Warning as WarningIcon } from '@/components/icons';
import { NODE_LAYER_STYLES, StyledBotGraphNode } from './styles';
import NodeTag from './NodeTag';
import NodeTooltip from './NodeTooltip';
import { useCallback } from 'react';

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
        <StyledBotGraphNode active={active} clickable={clickable} warn={!hasConfig} onClick={handleClick}>
            {/* Top Layer - Provider name and model display (same as ProviderNode) */}
            <Box sx={NODE_LAYER_STYLES.topLayer}>
                <NodeTooltip title={
                    hasConfig
                        ? <>Provider: {providerName || provider}<br/>Model: {model}</>
                        : 'Click to configure bot model'
                } placement="top">
                    {/* Provider and model share the row by content, not fixed
                        100px / 70px slots — those truncated any model id
                        longer than ~8 characters ("claude-s…"). The model
                        gets the room; the provider name yields first. */}
                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 0.5, width: '100%', minWidth: 0, px: 0.5 }}>
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
                            noWrap
                            sx={{
                                color: "text.primary",
                                ...NODE_LAYER_STYLES.typography,
                                fontStyle: !provider ? 'italic' : 'normal',
                                flex: '0 1 auto',
                                minWidth: 0,
                                maxWidth: provider ? '42%' : '100%',
                                textAlign: 'center'
                            }}>
                            {providerName || provider || 'select model'}
                        </Typography>

                        {provider && (
                            <Divider orientation="vertical" flexItem sx={{ mx: 0.5 }} />
                        )}

                        {provider && (
                            <Typography
                                variant="body2"
                                noWrap
                                sx={{
                                    color: "text.primary",
                                    ...NODE_LAYER_STYLES.typography,
                                    fontStyle: !model ? 'italic' : 'normal',
                                    flex: '0 1 auto',
                                    minWidth: 0,
                                    textAlign: 'center'
                                }}>
                                {model || 'select model'}
                            </Typography>
                        )}
                    </Box>
                </NodeTooltip>
            </Box>
            <Divider sx={NODE_LAYER_STYLES.divider} />
            {/* Bottom Layer - type tag */}
            <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                <NodeTag label="Model" active={active}/>
            </Box>
        </StyledBotGraphNode>
    );
};

export default BotModelNode;
