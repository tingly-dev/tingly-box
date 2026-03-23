import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import { getAllPlatforms } from '@/constants/platformGuides';
import type { BotSettings } from '@/types/bot';
import {
    ArrowForward,
    ChatBubble,
    CheckCircle,
    InfoOutlined,
    SettingsRemote,
} from '@mui/icons-material';
import {
    Box,
    Button,
    Card,
    CardActionArea,
    CardContent,
    Chip,
    Grid,
    Stack,
    Typography
} from '@mui/material';
import { useEffect, useMemo, useState } from 'react';
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

    // Get platforms from shared config and add bot counts
    const platforms = useMemo(() => {
        return getAllPlatforms().map(platform => ({
            ...platform,
            botCount: bots.filter(b => b.platform === platform.id).length,
        }));
    }, [bots]);

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

                {/* Platforms Section */}
                <UnifiedCard title="Supported Platforms" subtitle="Select a platform to configure" size="full">
                    <Grid container spacing={2}>
                        {platforms.map((platform) => (
                            <Grid key={platform.id} size={{ xs: 12, sm: 6, md: 4, lg: 3 }}>
                                <Card
                                    sx={{
                                        height: '100%',
                                        position: 'relative',
                                        opacity: platform.status === 'coming-soon' ? 0.6 : 1,
                                        cursor: platform.status === 'coming-soon' ? 'not-allowed' : 'pointer',
                                        transition: 'transform 0.2s, box-shadow 0.2s',
                                        '&:hover': platform.status !== 'coming-soon' ? {
                                            transform: 'translateY(-4px)',
                                            boxShadow: 4,
                                        } : {},
                                    }}
                                >
                                    <CardActionArea
                                        disabled={platform.status === 'coming-soon'}
                                        onClick={() => platform.status !== 'coming-soon' && navigate(platform.path)}
                                        sx={{ height: '100%', display: 'flex', flexDirection: 'column', alignItems: 'flex-start' }}
                                    >
                                        <CardContent sx={{ width: '100%', flexGrow: 1 }}>
                                            <Stack spacing={2}>
                                                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                                                    <Typography variant="h3">{platform.icon}</Typography>
                                                    {platform.status === 'coming-soon' && (
                                                        <Chip
                                                            label="Coming Soon"
                                                            size="small"
                                                            sx={{
                                                                height: 20,
                                                                fontSize: '0.65rem',
                                                                bgcolor: 'grey.100',
                                                                color: 'text.secondary',
                                                            }}
                                                        />
                                                    )}
                                                    {platform.status === 'beta' && (
                                                        <Chip
                                                            label="Beta"
                                                            size="small"
                                                            sx={{
                                                                height: 20,
                                                                fontSize: '0.65rem',
                                                                bgcolor: 'warning.lighter',
                                                                color: 'warning.dark',
                                                            }}
                                                        />
                                                    )}
                                                </Box>
                                                <Box>
                                                    <Typography variant="h6" sx={{ fontWeight: 600, mb: 0.5 }}>
                                                        {platform.name}
                                                    </Typography>
                                                    <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                                                        {platform.description}
                                                    </Typography>
                                                    {(platform.status === 'available' || platform.status === 'beta') && (
                                                        <Typography variant="caption" color="text.secondary">
                                                            {platform.botCount} bot{platform.botCount !== 1 ? 's' : ''} configured
                                                        </Typography>
                                                    )}
                                                </Box>
                                            </Stack>
                                        </CardContent>
                                        {(platform.status === 'available' || platform.status === 'beta') && (
                                            <Box sx={{ p: 1.5, pt: 0 }}>
                                                <Button
                                                    size="small"
                                                    endIcon={<ArrowForward />}
                                                    sx={{ width: '100%' }}
                                                >
                                                    Configure
                                                </Button>
                                            </Box>
                                        )}
                                    </CardActionArea>
                                </Card>
                            </Grid>
                        ))}
                    </Grid>
                </UnifiedCard>
            </Stack>
        </PageLayout>
    );
};

export default RemoteControlOverviewPage;
