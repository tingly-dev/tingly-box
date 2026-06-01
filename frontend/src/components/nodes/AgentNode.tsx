import { Box, Chip, Divider, Paper, Popover, Typography, styled } from '@mui/material';
import { NODE_LAYER_STYLES } from './styles';
import { useCallback, useRef, useState } from 'react';

type AgentType = 'claude-code' | 'smart-guide' | 'custom' | 'mock';

interface AgentInfo {
    description: string;
    features: string[];
    config: string;
}

const AGENT_TYPE_CONFIG: Record<AgentType, {
    label: string;
    color: 'info' | 'success' | 'default' | 'warning';
    info: AgentInfo;
}> = {
    'claude-code': {
        label: 'Claude Code',
        color: 'info',
        info: {
            description: 'Connects to the Claude Code CLI running on your local machine, enabling code generation, file editing, and terminal commands via IM.',
            features: [
                'Execute shell commands remotely',
                'Read and edit files in your project',
                'Run Claude Code tasks from any IM client',
                'Supports working directory isolation per bot',
            ],
            config: 'Click this node to open the Claude Code setup page and configure profiles.',
        },
    },
    'smart-guide': {
        label: 'SmartGuide',
        color: 'success',
        info: {
            description: 'An intelligent routing agent that processes messages through the configured LLM service, supporting smart rules, model selection, and context-aware responses.',
            features: [
                'Routes messages to any OpenAI-compatible or Anthropic provider',
                'Supports smart routing rules and priority tiers',
                'Context-aware conversation management',
                'Compatible with virtual models and guardrails',
            ],
            config: 'Click the Model node to the right to select the provider and model for this agent.',
        },
    },
    'custom': {
        label: 'Custom',
        color: 'warning',
        info: {
            description: 'A custom agent implementation with user-defined behavior and endpoints.',
            features: ['User-defined request/response handling', 'Custom tool integrations'],
            config: 'Configure via the agent settings panel.',
        },
    },
    'mock': {
        label: 'Mock',
        color: 'default',
        info: {
            description: 'A mock agent for testing and development purposes. Returns predefined responses.',
            features: ['Predefined test responses', 'No external API calls', 'Useful for UI testing'],
            config: 'No configuration required.',
        },
    },
};

const StyledAgentNode = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'active' && prop !== 'clickable',
})<{ active: boolean; clickable: boolean }>(({ active, clickable, theme }) => ({
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
    const config = AGENT_TYPE_CONFIG[agentType] ?? AGENT_TYPE_CONFIG['mock'];
    const displayLabel = label || config.label;
    const clickable = !!onClick;

    const anchorEl = useRef<HTMLDivElement | null>(null);
    const [open, setOpen] = useState(false);
    const enterTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

    const handleMouseEnter = useCallback(() => {
        enterTimer.current = setTimeout(() => setOpen(true), 400);
    }, []);

    const handleMouseLeave = useCallback(() => {
        if (enterTimer.current) clearTimeout(enterTimer.current);
        setOpen(false);
    }, []);

    return (
        <>
            <StyledAgentNode
                ref={anchorEl}
                active={active}
                clickable={clickable}
                onClick={onClick}
                onMouseEnter={handleMouseEnter}
                onMouseLeave={handleMouseLeave}
            >
                <Box sx={NODE_LAYER_STYLES.topLayer}>
                    <Typography variant="body2" sx={NODE_LAYER_STYLES.typography}>Agent</Typography>
                </Box>

                <Divider sx={NODE_LAYER_STYLES.divider} />

                <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                    <Chip
                        label={displayLabel}
                        size="small"
                        color={config.color as any}
                        sx={{ height: 24, fontSize: '0.75rem', fontWeight: 600 }}
                    />
                </Box>
            </StyledAgentNode>

            <Popover
                open={open}
                anchorEl={anchorEl.current}
                onClose={() => setOpen(false)}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
                transformOrigin={{ vertical: 'top', horizontal: 'center' }}
                disableRestoreFocus
                slotProps={{ paper: { onMouseEnter: handleMouseEnter, onMouseLeave: handleMouseLeave } }}
                sx={{ pointerEvents: 'none' }}
            >
                <Paper sx={{ p: 2, maxWidth: 300, pointerEvents: 'auto' }}>
                    {/* Header */}
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                        <Chip
                            label={config.label}
                            size="small"
                            color={config.color as any}
                            sx={{ fontWeight: 700, fontSize: '0.75rem' }}
                        />
                        <Typography variant="caption" color="text.secondary">Agent</Typography>
                    </Box>

                    {/* Description */}
                    <Typography variant="body2" sx={{ mb: 1.5, lineHeight: 1.55, color: 'text.primary' }}>
                        {config.info.description}
                    </Typography>

                    <Divider sx={{ mb: 1.5 }} />

                    {/* Features */}
                    <Typography variant="caption" sx={{ fontWeight: 700, color: 'text.secondary', textTransform: 'uppercase', letterSpacing: 0.5 }}>
                        Features
                    </Typography>
                    <Box component="ul" sx={{ m: 0, mt: 0.5, pl: 2.5, mb: 1.5 }}>
                        {config.info.features.map((f) => (
                            <Box component="li" key={f} sx={{ mb: 0.25 }}>
                                <Typography variant="caption" color="text.secondary">{f}</Typography>
                            </Box>
                        ))}
                    </Box>

                    <Divider sx={{ mb: 1.5 }} />

                    {/* Config hint */}
                    <Typography variant="caption" sx={{ fontWeight: 700, color: 'text.secondary', textTransform: 'uppercase', letterSpacing: 0.5 }}>
                        Configuration
                    </Typography>
                    <Typography variant="caption" display="block" sx={{ mt: 0.5, color: 'text.secondary', lineHeight: 1.5 }}>
                        {config.info.config}
                    </Typography>
                </Paper>
            </Popover>
        </>
    );
};

export default AgentNode;
