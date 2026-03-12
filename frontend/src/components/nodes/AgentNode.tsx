import { Box, Typography, styled, Chip, Divider } from '@mui/material';
import { NODE_LAYER_STYLES } from './styles';

const StyledAgentNode = styled(Box, { shouldForwardProp: (prop) => prop !== 'active' && prop !== 'clickable' })<{
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
    borderColor: active ? 'primary.main' : 'divider',
    backgroundColor: active ? 'primary.50' : 'background.paper',
    textAlign: 'center',
    width: 220,
    height: 90,
    boxShadow: theme.shadows[2],
    transition: 'all 0.2s ease-in-out',
    position: 'relative',
    opacity: active ? 1 : 0.6,
    cursor: clickable ? 'pointer' : 'default',
    ...(clickable && {
        '&:hover': {
            boxShadow: theme.shadows[4],
            transform: 'translateY(-2px)',
        },
    }),
}));

type AgentType = 'claude-code' | 'custom' | 'mock';

const AGENT_TYPE_LABELS: Record<AgentType, { label: string; color: 'info' | 'default' | 'warning' }> = {
    'claude-code': { label: 'Claude Code', color: 'info' },
    'custom': { label: 'Custom', color: 'warning' },
    'mock': { label: 'Mock', color: 'default' },
};

interface AgentNodeProps {
    agentType?: AgentType;
    active?: boolean;
    label?: string;
    onClick?: () => void;
}

const AgentNode: React.FC<AgentNodeProps> = ({
    agentType = 'claude-code',
    active = true,
    label,
    onClick,
}) => {
    const typeInfo = AGENT_TYPE_LABELS[agentType] || { label: 'Unknown', color: 'default' as const };
    const displayLabel = label || typeInfo.label;
    const clickable = !!onClick;

    return (
        <StyledAgentNode
            active={active}
            clickable={clickable}
            onClick={onClick}
        >
            <Box sx={NODE_LAYER_STYLES.topLayer}>
                <Typography variant="body2" sx={NODE_LAYER_STYLES.typography}>Agent</Typography>
            </Box>

            <Divider sx={NODE_LAYER_STYLES.divider} />

            <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                <Chip
                    label={displayLabel}
                    size="small"
                    color={typeInfo.color as any}
                    sx={{ height: 24, fontSize: '0.75rem', fontWeight: 600 }}
                />
            </Box>
        </StyledAgentNode>
    );
};

export default AgentNode;
