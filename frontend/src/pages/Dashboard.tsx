import { Box, Button, Card, CardContent, CardActions, Typography, alpha } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import OpenAI from '@lobehub/icons/es/OpenAI';
import Anthropic from '@lobehub/icons/es/Anthropic';
import Claude from '@lobehub/icons/es/Claude';
import { Settings as SystemIcon, Code as CodeIcon, BarChart as BarChartIcon, Lock as LockIcon } from '@mui/icons-material';
import ArrowForwardIcon from '@mui/icons-material/ArrowForward';
import { useTranslation } from 'react-i18next';
import { useFeatureFlags } from '../contexts/FeatureFlagsContext';
import { Send as UserPromptIcon, Bolt as SkillIcon } from '@mui/icons-material';

interface NavCard {
    title: string;
    description: string;
    path: string;
    icon: React.ReactNode;
    color: string;
}

interface CardGroup {
    categoryLabel: string;
    cards: NavCard[];
}

const Dashboard = () => {
    const navigate = useNavigate();
    const { t } = useTranslation();
    const { skillUser, skillIde } = useFeatureFlags();

    // Build prompt cards based on feature flags
    const promptCards: NavCard[] = [];
    if (skillUser) {
        promptCards.push({
            title: 'User Request',
            description: 'Manage user prompt templates',
            path: '/prompt/user',
            icon: <UserPromptIcon sx={{ fontSize: 40 }} />,
            color: '#9333ea',
        });
    }
    if (skillIde) {
        promptCards.push({
            title: 'Skills',
            description: 'Configure AI skills and commands',
            path: '/prompt/skill',
            icon: <SkillIcon sx={{ fontSize: 40 }} />,
            color: '#e11d48',
        });
    }

    const cardGroups: CardGroup[] = [
        {
            categoryLabel: 'Overview',
            cards: [
                {
                    title: 'Usage Dashboard',
                    description: 'View usage statistics and analytics',
                    path: '/dashboard',
                    icon: <BarChartIcon sx={{ fontSize: 40 }} />,
                    color: '#2563eb',
                },
            ],
        },
        {
            categoryLabel: 'AI Model Providers',
            cards: [
                {
                    title: t('layout.nav.useOpenAI', { defaultValue: 'OpenAI' }),
                    description: 'Use OpenAI SDK to visit models',
                    path: '/use-openai',
                    icon: <OpenAI size={40} />,
                    color: '#10a37f',
                },
                {
                    title: t('layout.nav.useAnthropic', { defaultValue: 'Anthropic' }),
                    description: 'Use Anthropic SDK to visit models',
                    path: '/use-anthropic',
                    icon: <Anthropic size={40} />,
                    color: '#D4915D',
                },
                {
                    title: t('layout.nav.useClaudeCode', { defaultValue: 'Claude Code' }),
                    description: 'Use Claude Code for AI Coding',
                    path: '/use-claude-code',
                    icon: <Claude style={{ fontSize: 40 }} />,
                    color: '#cc785c',
                },
                {
                    title: t('layout.nav.useOpenCode', { defaultValue: 'OpenCode' }),
                    description: 'Use OpenCode for development',
                    path: '/use-opencode',
                    icon: <CodeIcon sx={{ fontSize: 40 }} />,
                    color: '#f59e0b',
                },
            ],
        },
        ...(promptCards.length > 0 ? [{
            categoryLabel: 'Prompt',
            cards: promptCards,
        }] : []),
        {
            categoryLabel: 'Credentials',
            cards: [
                {
                    title: t('layout.nav.credentials', { defaultValue: 'All Credentials' }),
                    description: 'Manage API keys and OAuth configurations',
                    path: '/credentials',
                    icon: <LockIcon sx={{ fontSize: 40 }} />,
                    color: '#1976d2',
                },
            ],
        },
        {
            categoryLabel: 'System',
            cards: [
                {
                    title: 'System',
                    description: 'View system status and configuration',
                    path: '/system',
                    icon: <SystemIcon sx={{ fontSize: 40 }} />,
                    color: '#616161',
                },
            ],
        },
    ];

    // Flatten all cards into a single list
    const allCards: NavCard[] = cardGroups.flatMap(group => group.cards);

    return (
        <Box
            sx={{
                px: { xs: 3, sm: 4, md: 5, lg: 6 },
                py: { xs: 4, sm: 5, md: 6 },
                maxWidth: 1400,
                mx: 'auto',
            }}
        >
            {/* Header */}
            <Box sx={{ mb: { xs: 4, sm: 5, md: 6 }, textAlign: 'center' }}>
                <Typography
                    variant="h3"
                    sx={{
                        fontWeight: 700,
                        mb: 1.5,
                        letterSpacing: '-0.02em',
                        fontSize: { xs: '1.75rem', sm: '2rem', md: '2.25rem' },
                    }}
                >
                    Welcome to Tingly Box
                </Typography>
                <Typography
                    variant="body1"
                    color="text.secondary"
                    sx={{ fontSize: '1rem', lineHeight: 1.6 }}
                >
                    Your AI Model Proxy Toolbox
                </Typography>
            </Box>

            {/* Unified Cards Grid */}
            <Box
                sx={{
                    display: 'grid',
                    gridTemplateColumns: {
                        xs: '1fr',
                        sm: 'repeat(2, 1fr)',
                        md: 'repeat(3, 1fr)',
                    },
                    gap: { xs: 2, sm: 2.5, md: 3 },
                }}
            >
                {allCards.map((card) => (
                    <Card
                        key={card.path}
                        component="button"
                        onClick={() => navigate(card.path)}
                        aria-label={`Navigate to ${card.title}`}
                        sx={{
                            height: '100%',
                            display: 'flex',
                            flexDirection: 'column',
                            borderRadius: 3,
                            border: '1px solid',
                            borderColor: 'divider',
                            backgroundColor: 'background.paper',
                            boxShadow:
                                '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)',
                            transition: 'all 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
                            cursor: 'pointer',
                            textAlign: 'left',
                            '&:hover': {
                                transform: 'translateY(-6px)',
                                boxShadow:
                                    '0 10px 15px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -2px rgba(0, 0, 0, 0.05)',
                                borderColor: alpha(card.color, 0.3),
                            },
                            '&:active': {
                                transform: 'translateY(-4px)',
                            },
                            '&:focus-visible': {
                                outline: `2px solid ${alpha(card.color, 0.5)}`,
                                outlineOffset: '2px',
                            },
                            '@media (hover: none)': {
                                '&:hover': {
                                    transform: 'none',
                                    boxShadow:
                                        '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)',
                                },
                                '&:active': {
                                    transform: 'scale(0.98)',
                                    backgroundColor: alpha(card.color, 0.08),
                                },
                            },
                        }}
                    >
                        <CardContent
                            sx={{
                                flexGrow: 1,
                                p: { xs: 2.5, sm: 3 },
                            }}
                        >
                            <Box
                                sx={{
                                    display: 'flex',
                                    justifyContent: 'center',
                                    alignItems: 'center',
                                    width: { xs: 56, sm: 64 },
                                    height: { xs: 56, sm: 64 },
                                    borderRadius: '50%',
                                    mb: 2.5,
                                    backgroundColor: alpha(card.color, 0.08),
                                    color: card.color,
                                    transition: 'all 0.3s ease',
                                    '.MuiCard-root:hover &': {
                                        backgroundColor: alpha(card.color, 0.12),
                                        transform: 'scale(1.05)',
                                    },
                                }}
                            >
                                {card.icon}
                            </Box>
                            <Typography
                                variant="h6"
                                sx={{
                                    fontWeight: 600,
                                    mb: 1,
                                    letterSpacing: '-0.01em',
                                    textAlign: 'center',
                                }}
                            >
                                {card.title}
                            </Typography>
                            <Typography
                                variant="body2"
                                color="text.secondary"
                                sx={{ textAlign: 'center', lineHeight: 1.6 }}
                            >
                                {card.description}
                            </Typography>
                        </CardContent>
                        <CardActions sx={{ justifyContent: 'center', pb: 2.5, pt: 0 }}>
                            <Button
                                variant="outlined"
                                endIcon={
                                    <ArrowForwardIcon
                                        sx={{ fontSize: 16, transition: 'transform 0.2s' }}
                                    />
                                }
                                sx={{
                                    borderRadius: 2,
                                    fontWeight: 600,
                                    fontSize: '0.875rem',
                                    px: 2.5,
                                    py: 1,
                                    borderColor: alpha(card.color, 0.4),
                                    color: card.color,
                                    transition: 'all 0.2s ease',
                                    '&:hover': {
                                        borderColor: card.color,
                                        backgroundColor: alpha(card.color, 0.1),
                                        transform: 'translateX(2px)',
                                        boxShadow: `0 2px 8px ${alpha(card.color, 0.2)}`,
                                    },
                                    '&:hover .MuiButton-endIcon': {
                                        transform: 'translateX(4px)',
                                    },
                                    '&:focus-visible': {
                                        outline: `2px solid ${card.color}`,
                                        outlineOffset: '2px',
                                    },
                                }}
                            >
                                Open
                            </Button>
                        </CardActions>
                    </Card>
                ))}
            </Box>
        </Box>
    );
};

export default Dashboard;
