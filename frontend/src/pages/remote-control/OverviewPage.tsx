import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import { getAllPlatforms } from '@/constants/platformGuides';
import type { BotSettings } from '@/types/bot';
import {
    ArrowForward,
    CheckCircle,
    InfoOutlined,
    Lan,
} from '@mui/icons-material';
import {
    Box,
    Card,
    CardActionArea,
    CardContent,
    Chip,
    Grid,
    Stack,
    Typography,
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

    // Per-platform summary
    const platformSummaries = useMemo(() => {
        return getAllPlatforms().map(platform => {
            const platformBots = bots.filter(b => b.platform === platform.id);
            return {
                ...platform,
                totalBots: platformBots.length,
                activeBots: platformBots.filter(b => b.enabled).length,
            };
        });
    }, [bots]);

    // Only show platforms that are available/beta or have bots configured
    const visiblePlatforms = useMemo(() => {
        return platformSummaries.filter(
            p => p.status !== 'coming-soon' || p.totalBots > 0
        );
    }, [platformSummaries]);

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
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
                                        <Lan sx={{ color: 'white', fontSize: 24 }} />
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

                {/* Platform Summary */}
                <UnifiedCard title="Platforms" subtitle="Click to manage platform bots" size="full">
                    <Grid container spacing={2}>
                        {visiblePlatforms.map((platform) => (
                            <Grid key={platform.id} size={{ xs: 12, sm: 6, md: 4 }}>
                                <Card
                                    sx={{
                                        height: '100%',
                                        cursor: 'pointer',
                                        transition: 'transform 0.2s, box-shadow 0.2s',
                                        '&:hover': {
                                            transform: 'translateY(-2px)',
                                            boxShadow: 4,
                                        },
                                    }}
                                >
                                    <CardActionArea
                                        onClick={() => navigate(platform.path)}
                                        sx={{ height: '100%', display: 'flex', flexDirection: 'column', alignItems: 'flex-start' }}
                                    >
                                        <CardContent sx={{ width: '100%' }}>
                                            <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={2}>
                                                <Stack direction="row" alignItems="center" spacing={1.5}>
                                                    <Typography variant="h3">{platform.BrandIcon ? <platform.BrandIcon size={28} grayscale={false} /> : platform.icon}</Typography>
                                                    <Box>
                                                        <Stack direction="row" alignItems="center" spacing={1}>
                                                            <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                                                                {platform.name}
                                                            </Typography>
                                                            {platform.status === 'beta' && (
                                                                <Chip
                                                                    label="Beta"
                                                                    size="small"
                                                                    sx={{ height: 20, fontSize: '0.65rem', bgcolor: 'warning.lighter', color: 'warning.dark' }}
                                                                />
                                                            )}
                                                        </Stack>
                                                        <Typography variant="body2" color="text.secondary">
                                                            {platform.totalBots > 0
                                                                ? `${platform.activeBots}/${platform.totalBots} active`
                                                                : 'No bots'}
                                                        </Typography>
                                                    </Box>
                                                </Stack>
                                                <ArrowForward sx={{ color: 'text.secondary', fontSize: 20 }} />
                                            </Stack>
                                        </CardContent>
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
