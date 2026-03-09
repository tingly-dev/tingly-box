import {
    Router as RouterIcon
} from '@mui/icons-material';
import { Box, Divider, styled, ToggleButton, ToggleButtonGroup, Tooltip, Typography } from '@mui/material';
import { NODE_LAYER_STYLES } from './styles';

const StyledRoutingNode = styled(Box)(({ theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: 12,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: 'info.main',
    backgroundColor: 'info.50',
    textAlign: 'center',
    width: 220,
    height: 90,
    boxShadow: theme.shadows[2],
    transition: 'all 0.2s ease-in-out',
}));

interface RoutingModeNodeProps {
    mode: 'direct' | 'smart_guide';
    onModeChange?: (mode: 'direct' | 'smart_guide') => void;
    disabled?: boolean;
}

const RoutingModeNode: React.FC<RoutingModeNodeProps> = ({
    mode,
    onModeChange,
    disabled = false,
}) => {
    return (
        <StyledRoutingNode>
            {/* Top Layer - Icon and Title */}
            <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 1, width: '100%' }}>
                <RouterIcon sx={{ fontSize: 24, color: 'info.main' }} />
                <Typography
                    variant="body2"
                    sx={{
                        fontWeight: 600,
                        fontSize: '0.85rem',
                        color: 'text.primary',
                    }}
                >
                    Routing Mode
                </Typography>
            </Box>

            {/* Divider */}
            <Divider sx={{ width: '80%', my: 0.5 }} />

            {/* Bottom Layer - Toggle Buttons (horizontal) */}
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: 24, gap: 1, width: '100%' }}>
                <ToggleButtonGroup
                    value={mode}
                    exclusive
                    onChange={(_, newMode) => newMode && onModeChange?.(newMode)}
                    size="small"
                    disabled={disabled}
                    sx={{
                        '& .MuiToggleButtonGroup-grouped': {
                            margin: 2,
                            border: '1px solid',
                            borderRadius: 1,
                        },
                    }}
                >
                    <Tooltip title="Direct routing - send directly to agent" arrow>
                        <ToggleButton
                            value="direct"
                            sx={{
                                ...NODE_LAYER_STYLES.toggleButton,
                                flex: 1,
                                borderColor: 'text.primary',
                                '&.Mui-selected': {
                                    backgroundColor: mode === 'direct' ? 'secondary.main' : 'transparent',
                                    color: mode === 'direct' ? 'white' : 'text.primary',
                                    borderColor: 'text.primary',
                                },
                                '&:hover': {
                                    backgroundColor: mode === 'direct' ? 'secondary.dark' : 'action.hover',
                                },
                            }}
                        >
                            Direct
                        </ToggleButton>
                    </Tooltip>
                    <Tooltip title="Smart guide - route through guide agent" arrow>
                        <ToggleButton
                            value="smart_guide"
                            sx={{
                                ...NODE_LAYER_STYLES.toggleButton,
                                flex: 1,
                                borderColor: 'text.primary',
                                '&.Mui-selected': {
                                    backgroundColor: mode === 'smart_guide' ? 'success.main' : 'transparent',
                                    color: mode === 'smart_guide' ? 'white' : 'text.primary',
                                    borderColor: 'text.primary',
                                },
                                '&:hover': {
                                    backgroundColor: mode === 'smart_guide' ? 'success.dark' : 'action.hover',
                                },
                            }}
                        >
                            Smart
                        </ToggleButton>
                    </Tooltip>
                </ToggleButtonGroup>
            </Box>
        </StyledRoutingNode>
    );
};

export default RoutingModeNode;
