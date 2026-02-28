import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import type { BotSettings } from '@/types/bot';
import {
    ChatBubble,
    CheckCircle,
    InfoOutlined,
    SettingsRemote,
    SmartToy,
    AutoAwesome,
} from '@mui/icons-material';
import {
    Box,
    Card,
    CardContent,
    Grid,
    Stack,
    Typography
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';

const RemoteControlOverviewPage = () => {
    const navigate = useNavigate();
    const [bots, setBots] = useState<BotSettings[]>([]);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        loadBots();
    }, []);

    const loadBots = async () => {
        try {
            setLoading(true);
            const data = await api.getImBotSettingsList();
            if (data?.success && Array.isArray(data.settings)) {
                setBots(data.settings);
            }
        } catch (err) {
            console.error('Failed to load bot settings:', err);
        } finally {
            setLoading(false);
        }
    };

    const enabledBots = bots.filter(b => b.enabled);
    const disabledBots = bots.filter(b => !b.enabled);

    const features = [
        {
            icon: <ChatBubble sx={{ fontSize: 32, color: 'primary.main' }} />,
            title: 'IM Bot Integration',
            description: 'Connect Telegram, WeChat, and other messaging platforms to control your AI assistant remotely.',
        },
        {
            icon: <AutoAwesome sx={{ fontSize: 32, color: 'primary.main' }} />,
            title: 'Agent Control',
            description: 'Execute commands, manage sessions, and interact with AI agents from your favorite chat app.',
        },
        {
            icon: <SettingsRemote sx={{ fontSize: 32, color: 'primary.main' }} />,
            title: 'Remote Execution',
            description: 'Run bash commands, manage projects, and control your development environment remotely.',
        },
    ];

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                {/* Hero Section */}
                <UnifiedCard size="full">
                    <Box sx={{ textAlign: 'center', py: 4 }}>
                        <SettingsRemote sx={{ fontSize: 64, color: 'primary.main', mb: 2 }} />
                        <Typography variant="h4" sx={{ fontWeight: 600, mb: 1 }}>
                            Remote Control
                        </Typography>
                        <Typography variant="body1" color="text.secondary" sx={{ maxWidth: 600, mx: 'auto' }}>
                            Control your AI assistant from anywhere. Configure bots to enable chat-based
                            interactions with your development environment.
                        </Typography>
                    </Box>
                </UnifiedCard>

                {/* Stats Section */}
                <Grid container spacing={2}>
                    <Grid size={{ xs: 12, sm: 4 }}>
                        <Card sx={{ height: '100%' }}>
                            <CardContent>
                                <Stack direction="row" alignItems="center" spacing={2}>
                                    <Box
                                        sx={{
                                            width: 48,
                                            height: 48,
                                            borderRadius: 2,
                                            bgcolor: 'primary.main',
                                            display: 'flex',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                        }}
                                    >
                                        <ChatBubble sx={{ color: 'white', fontSize: 24 }} />
                                    </Box>
                                    <Box>
                                        <Typography variant="h4" sx={{ fontWeight: 600 }}>
                                            {bots.length}
                                        </Typography>
                                        <Typography variant="body2" color="text.secondary">
                                            Total Bots
                                        </Typography>
                                    </Box>
                                </Stack>
                            </CardContent>
                        </Card>
                    </Grid>
                    <Grid size={{ xs: 12, sm: 4 }}>
                        <Card sx={{ height: '100%' }}>
                            <CardContent>
                                <Stack direction="row" alignItems="center" spacing={2}>
                                    <Box
                                        sx={{
                                            width: 48,
                                            height: 48,
                                            borderRadius: 2,
                                            bgcolor: 'success.main',
                                            display: 'flex',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                        }}
                                    >
                                        <CheckCircle sx={{ color: 'white', fontSize: 24 }} />
                                    </Box>
                                    <Box>
                                        <Typography variant="h4" sx={{ fontWeight: 600 }}>
                                            {enabledBots.length}
                                        </Typography>
                                        <Typography variant="body2" color="text.secondary">
                                            Active
                                        </Typography>
                                    </Box>
                                </Stack>
                            </CardContent>
                        </Card>
                    </Grid>
                    <Grid size={{ xs: 12, sm: 4 }}>
                        <Card sx={{ height: '100%' }}>
                            <CardContent>
                                <Stack direction="row" alignItems="center" spacing={2}>
                                    <Box
                                        sx={{
                                            width: 48,
                                            height: 48,
                                            borderRadius: 2,
                                            bgcolor: 'grey.400',
                                            display: 'flex',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                        }}
                                    >
                                        <InfoOutlined sx={{ color: 'white', fontSize: 24 }} />
                                    </Box>
                                    <Box>
                                        <Typography variant="h4" sx={{ fontWeight: 600 }}>
                                            {disabledBots.length}
                                        </Typography>
                                        <Typography variant="body2" color="text.secondary">
                                            Inactive
                                        </Typography>
                                    </Box>
                                </Stack>
                            </CardContent>
                        </Card>
                    </Grid>
                </Grid>

                {/* Features Section */}
                <UnifiedCard title="Features" size="full">
                    <Grid container spacing={3}>
                        {features.map((feature, index) => (
                            <Grid key={index} size={{ xs: 12, md: 4 }}>
                                <Stack spacing={2} sx={{ textAlign: 'center', p: 2 }}>
                                    {feature.icon}
                                    <Typography variant="h6" sx={{ fontWeight: 600 }}>
                                        {feature.title}
                                    </Typography>
                                    <Typography variant="body2" color="text.secondary">
                                        {feature.description}
                                    </Typography>
                                </Stack>
                            </Grid>
                        ))}
                    </Grid>
                </UnifiedCard>
            </Stack>
        </PageLayout>
    );
};

export default RemoteControlOverviewPage;
