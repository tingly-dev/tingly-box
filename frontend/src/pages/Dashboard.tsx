import { Box, Button, Card, CardContent, CardActions, Typography } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import OpenAI from '@lobehub/icons/es/OpenAI';
import Anthropic from '@lobehub/icons/es/Anthropic';
import Claude from '@lobehub/icons/es/Claude';
import { Key as KeyIcon, VerifiedUser as OAuthIcon, Settings as SystemIcon } from '@mui/icons-material';
import { useTranslation } from 'react-i18next';

interface NavCard {
    title: string;
    description: string;
    path: string;
    icon: React.ReactNode;
    color: string;
}

const Dashboard = () => {
    const navigate = useNavigate();
    const { t } = useTranslation();

    const navCards: NavCard[] = [
        {
            title: t('layout.nav.useOpenAI', { defaultValue: 'OpenAI' }),
            description: 'Use OpenAI SDK to visit models',
            path: '/use-openai',
            icon: <OpenAI size={48} />,
            color: '#10a37f',
        },
        {
            title: t('layout.nav.useAnthropic', { defaultValue: 'Anthropic' }),
            description: 'Use Anthropic SDK to visit models',
            path: '/use-anthropic',
            icon: <Anthropic size={48} />,
            color: '#D4915D',
        },
        {
            title: t('layout.nav.useClaudeCode', { defaultValue: 'Claude Code' }),
            description: 'Use Claude Code for AI Coding',
            path: '/use-claude-code',
            icon: <Claude style={{ fontSize: 48 }} />,
            color: '#cc785c',
        },
        {
            title: t('layout.nav.apiKeys', { defaultValue: 'API Keys' }),
            description: 'Manage your API keys',
            path: '/api-keys',
            icon: <KeyIcon sx={{ fontSize: 48 }} />,
            color: '#1976d2',
        },
        {
            title: t('layout.nav.oauth', { defaultValue: 'OAuth' }),
            description: 'Configure OAuth authentication',
            path: '/oauth',
            icon: <OAuthIcon sx={{ fontSize: 48 }} />,
            color: '#2e7d32',
        },
        {
            title: 'System',
            description: 'View system status and configuration',
            path: '/system',
            icon: <SystemIcon sx={{ fontSize: 48 }} />,
            color: '#616161',
        },
    ];

    return (
        <Box
            sx={{
                p: 4,
                maxWidth: 1200,
                mx: 'auto',
            }}
        >
            {/* Header */}
            <Box sx={{ mb: 6, textAlign: 'center' }}>
                <Typography variant="h4" sx={{ fontWeight: 600, mb: 2 }}>
                    Welcome to Tingly Box
                </Typography>
                <Typography variant="body1" color="text.secondary">
                    Choose a function panel to get started
                </Typography>
            </Box>

            {/* Navigation Cards Grid */}
            <Box
                sx={{
                    display: 'grid',
                    gridTemplateColumns: {
                        xs: '1fr',
                        sm: 'repeat(2, 1fr)',
                        lg: 'repeat(3, 1fr)',
                    },
                    gap: 3,
                }}
            >
                {navCards.map((card) => (
                    <Card
                        key={card.path}
                        sx={{
                            height: '100%',
                            display: 'flex',
                            flexDirection: 'column',
                            transition: 'transform 0.2s, box-shadow 0.2s',
                            '&:hover': {
                                transform: 'translateY(-4px)',
                                boxShadow: 4,
                            },
                        }}
                    >
                        <CardContent sx={{ flexGrow: 1, textAlign: 'center' }}>
                            <Box
                                sx={{
                                    display: 'flex',
                                    justifyContent: 'center',
                                    alignItems: 'center',
                                    mb: 2,
                                    height: 64,
                                }}
                            >
                                <Box sx={{ color: card.color }}>
                                    {card.icon}
                                </Box>
                            </Box>
                            <Typography variant="h6" sx={{ fontWeight: 600, mb: 1 }}>
                                {card.title}
                            </Typography>
                            <Typography variant="body2" color="text.secondary">
                                {card.description}
                            </Typography>
                        </CardContent>
                        <CardActions sx={{ justifyContent: 'center', pb: 2 }}>
                            <Button
                                variant="outlined"
                                onClick={() => navigate(card.path)}
                                sx={{
                                    borderColor: card.color,
                                    color: card.color,
                                    '&:hover': {
                                        borderColor: card.color,
                                        backgroundColor: `${card.color}08`,
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
