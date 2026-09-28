import {
    Alert,
    Box,
    Card,
    CardActionArea,
    Chip,
    Grid,
    Stack,
    Switch,
    Tooltip,
    Typography,
    alpha,
} from '@mui/material';
import {
    Bolt as IconBolt,
    Code as IconCode,
    Send as IconSend,
    RemoteControl as IconRemote,
    Shield as IconShield,
    TestPipe as IconTestPipe,
    Handyman as IconTools,
} from '@/components/icons';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import type { ExperimentalFeature } from '@/components/ExperimentalFeatureGate';
import { useFeatureFlags } from '@/contexts/FeatureFlagsContext';
import { api } from '@/services/api';
import { isFullEdition } from '@/utils/edition';
import { useBotPlatformSummary } from '@/layout/useBotPlatformSummary';
import { useHiddenScenarios } from './scenarioRegistry';
import UnifiedCard from '@/components/UnifiedCard';

interface PowerUp {
    key: string;
    // The `_global` flag behind the switch. Absent = the switch only shows/hides
    // the rail item (onToggle) and the feature itself keeps running.
    feature?: ExperimentalFeature;
    onToggle?: () => void;
    icon: React.ReactNode;
    name: string;
    description: string;
    path: string;
    enabled: boolean;
    // Shown under the description while the power-up is on — only for ones
    // whose "on" state widens who can do what on this machine.
    enabledNotice?: string;
    // Live status line under the name (e.g. bot count), like the agent cards' rule count.
    status?: string;
    // Still experimental (vs. the other power-ups, which are just opt-in) —
    // badged so users know what they're turning on.
    experimental?: boolean;
}

// Power-ups — opt-in features that extend an agent, controlled in
// place on the agent overview instead of only under System → Experimental.
// Same `_global` flags as that page (which stays as the full list and the
// ExperimentalFeatureGate landing spot); turning one on adds its rail item,
// and the card then opens it.
const PowerUpsSection: React.FC = () => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { skillUser, skillIde, enableGuardrails, enableMCP, enableBench, enableDesk, loading, refresh } = useFeatureFlags();
    const [updating, setUpdating] = useState<ExperimentalFeature>();
    const [failed, setFailed] = useState(false);
    const { isHidden, toggleHidden } = useHiddenScenarios();
    const botSummary = useBotPlatformSummary(isFullEdition);
    const botTotals = Object.values(botSummary).reduce(
        (acc, b) => ({ active: acc.active + b.active, total: acc.total + b.total }),
        { active: 0, total: 0 },
    );

    const iconSx = { fontSize: 24, color: 'text.secondary' };
    const powerUps: PowerUp[] = [
        // Remote leads: the established power-up (drive agents from IM). Not
        // flag-gated (full edition only) — its switch hides/shows the rail item
        // via the same hidden set as Team/Image; connected bots keep running.
        ...(isFullEdition ? [{
            key: 'remote',
            icon: <IconRemote sx={iconSx} />,
            name: t('layout.remote'),
            description: t('scenarioOverview.powerUps.remoteDesc', { defaultValue: 'Drive your agents from IM — connect bots for remote control and notifications.' }),
            path: '/bots/overview',
            enabled: !isHidden('remote'),
            onToggle: () => toggleHidden('remote'),
            status: botTotals.total > 0
                ? t('bots.activeCount', { defaultValue: 'active {{active}} / {{total}}', active: botTotals.active, total: botTotals.total })
                : undefined,
        }] : []),
        {
            key: 'bench',
            feature: 'bench',
            icon: <IconTestPipe sx={iconSx} />,
            name: t('system.experimentalFeatures.bench'),
            description: t('system.experimentalFeatures.enableBench'),
            path: '/bench',
            enabled: enableBench,
        },
        // Desk lives under Remote in the rail, which is full-edition only.
        ...(isFullEdition ? [{
            key: 'desk',
            feature: 'desk' as const,
            icon: <IconCode sx={iconSx} />,
            name: t('system.experimentalFeatures.desk', { defaultValue: 'Desk' }),
            description: t('system.experimentalFeatures.enableDesk', { defaultValue: 'Run Claude Code on this machine from a browser tab, in a folder you choose.' }),
            path: '/desk',
            enabled: enableDesk,
            enabledNotice: t('system.experimentalFeatures.deskEnabledInfo', { defaultValue: 'Anyone who can sign in to this tingly-box can now start Claude Code sessions on this machine and approve the tool calls they make.' }),
        }] : []),
        {
            key: 'mcp',
            feature: 'mcp',
            icon: <IconTools sx={iconSx} />,
            name: `${t('system.experimentalFeatures.mcp')} Tools`,
            description: t('system.experimentalFeatures.enableMCP'),
            path: '/mcp/sources',
            enabled: enableMCP,
            experimental: true,
        },
        {
            key: 'guardrails',
            feature: 'guardrails',
            icon: <IconShield sx={iconSx} />,
            name: t('system.experimentalFeatures.guardrails'),
            description: t('system.experimentalFeatures.enableGuardrails'),
            path: '/guardrails',
            enabled: enableGuardrails,
            experimental: true,
        },
        ...(isFullEdition ? [
            {
                key: 'skill_user',
            feature: 'skill_user' as const,
                icon: <IconSend sx={iconSx} />,
                name: t('system.experimentalFeatures.userPrompts'),
                description: t('system.experimentalFeatures.enableUserPrompts'),
                path: '/prompt/user',
                enabled: skillUser,
                experimental: true,
            },
            {
                key: 'skill_ide',
            feature: 'skill_ide' as const,
                icon: <IconBolt sx={iconSx} />,
                name: t('system.experimentalFeatures.skills'),
                description: t('system.experimentalFeatures.enableIdeSkills'),
                path: '/prompt/skill',
                enabled: skillIde,
                experimental: true,
            },
        ] : []),
    ];

    const toggle = async (p: PowerUp) => {
        if (p.onToggle) return p.onToggle();
        if (!p.feature) return;
        setFailed(false);
        setUpdating(p.feature);
        try {
            const result = await api.setScenarioFlag('_global', p.feature, !p.enabled);
            if (!result.success) throw new Error('Feature update was rejected');
        } catch (error) {
            console.error(`Failed to set ${p.feature}:`, error);
            setFailed(true);
        } finally {
            await refresh();
            setUpdating(undefined);
        }
    };

    return (
        <UnifiedCard
            size="full"
            title={t('scenarioOverview.powerUps.title', { defaultValue: 'Power-ups' })}
            subtitle={t('scenarioOverview.powerUps.subtitle', { defaultValue: 'Optional capabilities that extend your agents. Turn one on to add it to the sidebar.' })}
        >

            {failed && (
                <Alert severity="error" sx={{ mb: 2 }}>
                    {t('system.experimentalFeatures.enableFailed')}
                </Alert>
            )}

            <Grid container spacing={2}>
                {powerUps.map((p) => (
                    <Grid key={p.key} size={{ xs: 12, sm: 6, md: 4, lg: 3 }}>
                        <Card
                            variant="outlined"
                            sx={{
                                position: 'relative',
                                height: '100%',
                                boxShadow: 'none',
                                transition: 'border-color 0.15s, background-color 0.15s',
                                '&:hover': p.enabled ? {
                                    borderColor: 'primary.main',
                                    bgcolor: (theme) => alpha(theme.palette.primary.main, 0.04),
                                } : undefined,
                            }}
                        >
                            {/* The switch stays visible (not hover-revealed like the
                                scenario visibility toggle): on/off is this card's state. */}
                            {(p.feature || p.onToggle) && (
                                <Switch
                                    size="small"
                                    checked={p.enabled}
                                    disabled={loading || updating !== undefined}
                                    onChange={() => toggle(p)}
                                    slotProps={{ input: { 'aria-label': p.name } }}
                                    // Vertically centred on the 36px title row (12px padding + 18 − 12).
                                    sx={{ position: 'absolute', top: 18, right: 8, zIndex: 1 }}
                                />
                            )}
                            <CardActionArea
                                disabled={!p.enabled}
                                onClick={() => navigate(p.path)}
                                sx={{ p: 1.5, height: '100%', alignItems: 'flex-start' }}
                            >
                                <Stack direction="row" spacing={1.25} sx={{ alignItems: 'center', mb: 0.75, pr: 5 }}>
                                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', width: 36, height: 36, flexShrink: 0 }}>
                                        {p.icon}
                                    </Box>
                                    {/* Same structure as the agent cards: name (+ Exp. tag) on
                                        top, status line under it — Enabled/Disabled, or a live
                                        status (Remote's bot count) — so the grid reads uniformly. */}
                                    <Box sx={{ flex: 1, minWidth: 0 }}>
                                        <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                                            <Typography variant="subtitle1" noWrap sx={{ fontWeight: 600, lineHeight: 1.2 }}>
                                                {p.name}
                                            </Typography>
                                            {p.experimental && (
                                                // Abbreviated so name + tag + switch fit one row;
                                                // the tooltip spells it out.
                                                <Tooltip title={t('scenarioOverview.powerUps.experimentalTooltip', { defaultValue: 'Experimental feature' })} arrow>
                                                    <Chip
                                                        size="small"
                                                        color="warning"
                                                        variant="outlined"
                                                        label={t('scenarioOverview.powerUps.experimental', { defaultValue: 'Exp.' })}
                                                        sx={{ height: 18, fontSize: '0.6875rem', flexShrink: 0 }}
                                                    />
                                                </Tooltip>
                                            )}
                                        </Stack>
                                        <Box sx={{ minHeight: 18, display: 'flex', alignItems: 'center' }}>
                                            <Typography variant="caption" sx={p.enabled ? { color: 'success.main', fontWeight: 500 } : { color: 'text.disabled' }}>
                                                {p.enabled ? (p.status ?? t('common.enabled')) : t('common.disabled')}
                                            </Typography>
                                        </Box>
                                    </Box>
                                </Stack>
                                <Typography
                                    variant="body2"
                                    sx={{
                                        color: 'text.secondary',
                                        minHeight: 40,
                                        display: '-webkit-box',
                                        WebkitLineClamp: 2,
                                        WebkitBoxOrient: 'vertical',
                                        overflow: 'hidden',
                                    }}
                                >
                                    {p.description}
                                </Typography>
                                {p.enabled && p.enabledNotice && (
                                    <Typography variant="caption" component="p" sx={{ color: 'warning.main', mt: 1 }}>
                                        {p.enabledNotice}
                                    </Typography>
                                )}
                            </CardActionArea>
                        </Card>
                    </Grid>
                ))}
            </Grid>
        </UnifiedCard>
    );
};

export default PowerUpsSection;
