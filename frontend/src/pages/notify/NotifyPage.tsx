import EmptyStateGuide from '@/components/EmptyStateGuide';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import { isNotifyMounted, notifyRoutes } from '@/types/bot';
import type { BotSettings } from '@/types/bot';
import { Box, Chip, CircularProgress, Stack, Tooltip, Typography } from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

// NotifyPage is the Notify purpose — the twin of Remote, but lighter: notify
// has no bot-config knobs of its own (see consumer_notify.go), it's mounted
// implicitly by whichever outbound scenario routes (claude_code, ...) point
// at a bot. That routing has no frontend surface yet, so this page is
// read-only for now: it shows the real mount status and routes derived from
// each bot's scenarios JSON, but attaching a NEW route needs a backend API +
// swagger definition this pass didn't add (see .design/bot-arch.md §10).
const NotifyPage = () => {
    const { t } = useTranslation();
    const [bots, setBots] = useState<BotSettings[]>([]);
    const [loading, setLoading] = useState(true);

    const loadBots = useCallback(async () => {
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
    }, []);

    useEffect(() => {
        loadBots();
    }, [loadBots]);

    return (
        <PageLayout loading={false}>
            <UnifiedCard
                title={t('notify.title', { defaultValue: 'Notify' })}
                subtitle={t('notify.subtitle', {
                    defaultValue: 'Which of your bots deliver scenario notifications and interactive prompts to chat.',
                })}
                size="full"
                sx={{ mb: 2 }}
            >
                {loading ? (
                    <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
                        <CircularProgress />
                    </Box>
                ) : bots.length === 0 ? (
                    <EmptyStateGuide
                        title={t('notify.emptyTitle', { defaultValue: 'No bots connected yet' })}
                        description={t('notify.emptyDescription', { defaultValue: 'Connect a bot on the Overview page first, then come back here to see what it notifies.' })}
                        showHeroIcon={false}
                    />
                ) : (
                    <Stack spacing={1}>
                        {bots.map((bot) => {
                            const mounted = isNotifyMounted(bot.scenarios);
                            const routes = notifyRoutes(bot.scenarios);
                            return (
                                <Box
                                    key={bot.uuid}
                                    sx={{
                                        display: 'flex',
                                        alignItems: 'center',
                                        flexWrap: 'wrap',
                                        gap: 1.5,
                                        px: 2,
                                        py: 1.5,
                                        border: '1px solid',
                                        borderColor: 'divider',
                                        borderRadius: 1.5,
                                    }}
                                >
                                    <Typography sx={{ fontFamily: 'monospace', fontSize: '0.875rem', fontWeight: 600 }}>
                                        {bot.name || bot.platform}
                                    </Typography>
                                    <Chip label={bot.platform} size="small" />
                                    <Chip
                                        label={mounted
                                            ? t('notify.mounted', { defaultValue: 'Notifying' })
                                            : t('notify.notMounted', { defaultValue: 'No routes' })}
                                        size="small"
                                        variant={mounted ? 'filled' : 'outlined'}
                                        color={mounted ? 'primary' : 'default'}
                                    />
                                    {routes.length > 0 && (
                                        <Typography variant="caption" sx={{ color: 'text.secondary', fontFamily: 'monospace' }}>
                                            {routes.map((r) => r.name).join(', ')}
                                        </Typography>
                                    )}
                                    <Box sx={{ flexGrow: 1 }} />
                                    <Tooltip title={t('notify.attachComingSoonHint', { defaultValue: 'Attaching a route from here is not wired up yet — routes are currently configured per scenario.' })}>
                                        <span>
                                            <Chip
                                                label={t('notify.attachComingSoon', { defaultValue: '+ Attach a route (soon)' })}
                                                size="small"
                                                disabled
                                            />
                                        </span>
                                    </Tooltip>
                                </Box>
                            );
                        })}
                    </Stack>
                )}
            </UnifiedCard>
        </PageLayout>
    );
};

export default NotifyPage;
