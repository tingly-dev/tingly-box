import { Box, Card, CardContent, Typography, Tooltip, Divider } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
    Claude,
    Codex,
    OpenAI as OpenAIIcon,
    Anthropic as AnthropicIcon,
    OpenCode,
    Xcode,
    VSCode,
    OpenClaw,
} from '../BrandIcons';

interface AgentItem {
    path: string;
    label: string;
    icon: React.ReactNode;
    description: string;
}

const AgentQuickNav: React.FC = () => {
    const { t } = useTranslation();
    const navigate = useNavigate();

    const agents: AgentItem[] = [
        {
            path: '/use-claude-code',
            label: 'Claude Code',
            icon: <Claude size={20} />,
            description: 'AI-powered coding assistant',
        },
        {
            path: '/use-codex',
            label: 'Codex',
            icon: <Codex size={20} />,
            description: 'OpenAI Codex integration',
        },
        {
            path: '/use-opencode',
            label: 'OpenCode',
            icon: <OpenCode size={20} />,
            description: 'OpenCode agent',
        },
        {
            path: '/use-xcode',
            label: 'Xcode',
            icon: <Xcode size={20} />,
            description: 'Xcode integration',
        },
        {
            path: '/use-vscode',
            label: 'VS Code',
            icon: <VSCode size={20} />,
            description: 'VS Code integration',
        },
        {
            path: '/use-openai',
            label: 'OpenAI',
            icon: <OpenAIIcon size={20} />,
            description: 'OpenAI SDK',
        },
        {
            path: '/use-anthropic',
            label: 'Anthropic',
            icon: <AnthropicIcon size={20} />,
            description: 'Anthropic SDK',
        },
        {
            path: '/use-agent',
            label: 'OpenClaw',
            icon: <OpenClaw size={20} />,
            description: 'Advanced agent framework',
        },
    ];

    return (
        <Card
            sx={{
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
                boxShadow: '0 2px 8px rgba(0, 0, 0, 0.08)',
                height: '100%',
                display: 'flex',
                flexDirection: 'column',
            }}
        >
            <CardContent sx={{ p: 2, height: '100%', display: 'flex', flexDirection: 'column' }}>
                {/* Header */}
                <Box sx={{ mb: 1.5 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, fontSize: '0.8rem' }}>
                        {t('dashboard.agentNav.title', { defaultValue: 'Quick Start' })}
                    </Typography>
                    <Typography variant="caption" sx={{ color: 'text.secondary', fontSize: '0.7rem' }}>
                        {t('dashboard.agentNav.description', { defaultValue: 'Jump to agent' })}
                    </Typography>
                </Box>

                <Divider sx={{ mb: 1.5 }} />

                {/* Agent List */}
                <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 0.5 }}>
                    {agents.map((agent) => (
                        <Tooltip
                            key={agent.path}
                            title={agent.description}
                            arrow
                            placement="right"
                        >
                            <Box
                                onClick={() => navigate(agent.path)}
                                sx={{
                                    display: 'flex',
                                    alignItems: 'center',
                                    gap: 1,
                                    py: 1.25,
                                    px: 1,
                                    borderRadius: 1.25,
                                    cursor: 'pointer',
                                    transition: 'all 0.2s ease',
                                    border: '1px solid transparent',
                                    position: 'relative',
                                    color: 'text.secondary',
                                    '&:hover': {
                                        bgcolor: 'primary.main',
                                        color: 'primary.contrastText',
                                        borderColor: 'primary.light',
                                        transform: 'translateX(4px)',
                                        boxShadow: '0 2px 8px rgba(37, 99, 235, 0.3)',
                                        '& .MuiTypography-root': {
                                            color: 'primary.contrastText',
                                        },
                                        '& svg': {
                                            filter: 'none !important',
                                        },
                                    },
                                }}
                            >
                                {/* Icon */}
                                <Box
                                    sx={{
                                        display: 'flex',
                                        alignItems: 'center',
                                        justifyContent: 'center',
                                        width: 32,
                                        height: 32,
                                        borderRadius: 1,
                                        bgcolor: 'action.hover',
                                        flexShrink: 0,
                                    }}
                                >
                                    {agent.icon}
                                </Box>

                                {/* Label */}
                                <Typography
                                    variant="caption"
                                    sx={{
                                        fontWeight: 500,
                                        fontSize: '0.75rem',
                                        color: 'text.primary',
                                        flex: 1,
                                        lineHeight: 1.3,
                                    }}
                                >
                                    {agent.label}
                                </Typography>
                            </Box>
                        </Tooltip>
                    ))}
                </Box>
            </CardContent>
        </Card>
    );
};

export default AgentQuickNav;
