import {Box, Chip, Divider, styled, Typography} from '@mui/material';
import type {BotSettings} from '@/types/bot';
import {NODE_LAYER_STYLES} from './styles';

const StyledImBotNode = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'active',
})<{ active: boolean }>(({active, theme}) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: 12,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: active ? 'success.main' : 'divider',
    backgroundColor: active ? 'success.50' : 'background.paper',
    textAlign: 'center',
    width: 220,
    height: 90,
    boxShadow: theme.shadows[2],
    transition: 'all 0.2s ease-in-out',
    position: 'relative',
    opacity: active ? 1 : 0.6,
    cursor: 'default',
}));

interface ImBotNodeProps {
    imbot: BotSettings;
    active?: boolean;
}

const ImBotNode: React.FC<ImBotNodeProps> = ({imbot, active = true}) => {
    const platformType = imbot.platform.toUpperCase();
    const displayName = imbot.name || 'Bot';

    return (
        <StyledImBotNode active={active}>
            {/* Top Layer - Platform Type | Name (like ProviderNode) */}
            <Box sx={NODE_LAYER_STYLES.topLayer}>
                <Box sx={{display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 0.5}}>
                    <Typography
                        variant="body2"
                        sx={{
                            ...NODE_LAYER_STYLES.typography,
                            fontStyle: 'normal',
                            width: '80px',
                            textAlign: 'center',
                            color: 'text.secondary',
                            fontSize: '0.75rem',
                            textTransform: 'uppercase',
                        }}
                    >
                        {platformType}
                    </Typography>

                    <Divider orientation="vertical" flexItem sx={{mx: 0.5}}/>

                    <Typography
                        variant="body2"
                        sx={{
                            ...NODE_LAYER_STYLES.typography,
                            fontStyle: !imbot.name ? 'italic' : 'normal',
                            width: '80px',
                            textAlign: 'center',
                            color: active ? 'text.primary' : 'text.disabled',
                        }}
                    >
                        {displayName || 'Set Name'}
                    </Typography>
                </Box>
            </Box>

            <Divider sx={NODE_LAYER_STYLES.divider}/>

            {/* Bottom Layer - Enable Chip (restored) */}
            <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                <Chip
                    label={active ? 'Enabled' : 'Disabled'}
                    size="small"
                    color={active ? 'success' : 'default'}
                    sx={{height: 24, fontSize: '0.7rem', fontWeight: 600}}
                />
            </Box>
        </StyledImBotNode>
    );
};

export default ImBotNode;
