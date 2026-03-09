import type { AgentConfig } from '@/types/remoteGraph';
import { CheckCircle as CheckCircleIcon, Settings as SettingsIcon } from '@mui/icons-material';
import { Box, Divider, styled } from '@mui/material';
import { NODE_LAYER_STYLES } from './styles';

const StyledConfigNode = styled(Box)(({ theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: 8,
    borderRadius: theme.shape.borderRadius,
    border: '1px dashed',
    borderColor: 'divider',
    backgroundColor: 'background.paper',
    textAlign: 'center',
    width: 180,
    height: 60,
    boxShadow: theme.shadows[1],
    transition: 'all 0.2s ease-in-out',
    position: 'relative',
    cursor: 'pointer',
    '&:hover': {
        borderColor: 'primary.main',
        boxShadow: theme.shadows[2],
        transform: 'translateY(-1px)',
    },
}));

interface AgentConfigNodeProps {
    agentConfig: AgentConfig;
    configured?: boolean;
    onClick?: () => void;
}

const AgentConfigNode: React.FC<AgentConfigNodeProps> = ({ agentConfig, configured = false, onClick }) => {
    return (
        <StyledConfigNode onClick={onClick}>
            <Box sx={NODE_LAYER_STYLES.topLayer}>
                <Box sx={{ position: 'relative' }}>
                    <SettingsIcon sx={{ fontSize: 24, color: configured ? 'primary.main' : 'text.disabled' }} />
                    {configured && (
                        <CheckCircleIcon sx={{
                            position: 'absolute',
                            bottom: -4,
                            right: -4,
                            fontSize: 12,
                            color: 'success.main',
                            backgroundColor: 'background.paper',
                            borderRadius: '50%',
                        }} />
                    )}
                </Box>
            </Box>

            <Divider sx={NODE_LAYER_STYLES.divider} />

            <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                <Box
                    component="span"
                    sx={{
                        fontSize: '0.7rem',
                        fontWeight: 600,
                        color: configured ? 'text.primary' : 'text.secondary',
                    }}
                >
                    {configured ? 'Configured' : 'Configure'}
                </Box>
            </Box>
        </StyledConfigNode>
    );
};

export default AgentConfigNode;
