import { Box, Typography, styled, Divider, Switch, Chip } from '@mui/material';
import type { BotSettings } from '@/types/bot';
import { NODE_LAYER_STYLES } from './styles';
import { useCallback } from 'react';

const StyledImBotNode = styled(Box, { shouldForwardProp: (prop) => prop !== 'active' })<{
    active: boolean;
}>(({ active, theme }) => ({
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
    onToggle?: (enabled: boolean) => void;
    isToggling?: boolean;
}

const ImBotNode: React.FC<ImBotNodeProps> = ({ imbot, active = true, onToggle, isToggling = false }) => {
    const displayName = imbot.name || imbot.platform || 'Bot';

    const handleToggle = useCallback((event: React.ChangeEvent<HTMLInputElement>, checked: boolean) => {
        event.stopPropagation();
        onToggle?.(checked);
    }, [onToggle]);

    return (
        <StyledImBotNode active={active}>
            <Box sx={NODE_LAYER_STYLES.topLayer}>
                <Typography variant="body2" sx={{
                    fontWeight: 600,
                    fontSize: '0.9rem',
                    color: 'text.primary',
                }}>
                    {displayName}
                </Typography>
            </Box>

            <Divider sx={NODE_LAYER_STYLES.divider} />

            <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                <Switch
                    checked={active}
                    onChange={handleToggle}
                    size="small"
                    disabled={isToggling}
                    sx={{
                        '& .MuiSwitch-switchBase.Mui-checked': { color: 'success.main' },
                        '& .MuiSwitch-switchBase.Mui-checked + .MuiSwitch-track': { backgroundColor: 'success.main' },
                    }}
                />
                <Chip
                    label={active ? 'Enabled' : 'Disabled'}
                    size="small"
                    color={active ? 'success' : 'default'}
                    sx={{ height: 20, fontSize: '0.65rem', fontWeight: 600 }}
                />
            </Box>
        </StyledImBotNode>
    );
};

export default ImBotNode;
