import {
    Alert,
    Box,
    Chip,
    Divider,
    ListItemButton,
    Paper,
    Popper,
    Stack,
    Switch,
    Tooltip,
    Typography,
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
import { useBotPlatformSummary } from './useBotPlatformSummary';
import { SCENARIOS, useHiddenScenarios } from '@/pages/scenario/scenarioRegistry';
import { Z_INDEX } from '../constants/zIndex';

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
    // Shown as a warning under the list while the power-up is on — only for
    // ones whose "on" state widens who can do what on this machine.
    enabledNotice?: string;
    // Live status next to the name (e.g. bot count).
    status?: string;
    // Maturity tag beside the name so users know what they're turning on:
    // 'exp' = experimental, 'beta' = usable but still evolving. Absent = stable.
    stage?: 'exp' | 'beta';
}

interface PowerUpsMenuProps {
    /** The user-menu row it opens beside; null = closed. */
    anchorEl: HTMLElement | null;
    onClose: () => void;
    /** Called after a row opens its page, so the parent menu can close too. */
    onNavigate?: () => void;
    /** Hover bookkeeping, so the parent keeps it open while the pointer is on it. */
    onMouseEnter?: () => void;
    onMouseLeave?: () => void;
}

// Power-ups — optional rail items (Team, Image, Remote, Bench, …), switched
// on and off from a submenu of the user menu (PreferencesMenu), next to the
// other set-once choices about how the app looks. Flag-backed ones use
// the same `_global` flags as System → Experimental (which stays as the full
// list and the ExperimentalFeatureGate landing spot). An enabled row opens
// its page.
export const PowerUpsMenu: React.FC<PowerUpsMenuProps> = ({ anchorEl, onClose, onNavigate, onMouseEnter, onMouseLeave }) => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { skillUser, skillIde, enableGuardrails, enableMCP, enableBench, enableDesk, loading, refresh } = useFeatureFlags();
    const [updating, setUpdating] = useState<ExperimentalFeature>();
    const [failed, setFailed] = useState(false);
    const { isHidden, toggleHidden } = useHiddenScenarios();
    const botSummary = useBotPlatformSummary();
    const botTotals = Object.values(botSummary).reduce(
        (acc, b) => ({ active: acc.active + b.active, total: acc.total + b.total }),
        { active: 0, total: 0 },
    );

    const iconSx = { fontSize: 20, color: 'text.secondary' };
    const powerUps: PowerUp[] = [
        // Team and Image are rail items switched through the hidden-scenario
        // set (the same one the Agent sidebar's edit mode uses); they used to
        // be cards on the /agent page.
        ...(['team', 'imagegen'] as const).flatMap((id) => {
            const s = SCENARIOS.find((x) => x.id === id);
            return s ? [{
                key: id,
                icon: s.icon(20),
                name: t(id === 'team' ? 'layout.team' : 'layout.image'),
                description: t(s.descKey),
                path: id === 'team' ? '/agent/team' : '/image/playground',
                enabled: !isHidden(id),
                onToggle: () => toggleHidden(id),
            }] : [];
        }),
        // Remote leads: the established power-up (drive agents from IM). Not
        // flag-gated — its switch hides/shows the rail item via the same
        // hidden set as Team/Image; connected bots keep running.
        {
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
        },
        {
            key: 'bench',
            feature: 'bench',
            icon: <IconTestPipe sx={iconSx} />,
            name: t('system.experimentalFeatures.bench'),
            description: t('system.experimentalFeatures.enableBench'),
            path: '/bench',
            stage: 'beta' as const,
            enabled: enableBench,
        },
        {
            key: 'desk',
            feature: 'desk' as const,
            icon: <IconCode sx={iconSx} />,
            name: t('system.experimentalFeatures.desk', { defaultValue: 'Desk' }),
            description: t('system.experimentalFeatures.enableDesk', { defaultValue: 'Run Claude Code on this machine from a browser tab, in a folder you choose.' }),
            path: '/desk',
            stage: 'beta' as const,
            enabled: enableDesk,
            enabledNotice: t('system.experimentalFeatures.deskEnabledInfo', { defaultValue: 'Anyone who can sign in to this tingly-box can now start Claude Code sessions on this machine and approve the tool calls they make.' }),
        },
        {
            key: 'mcp',
            feature: 'mcp',
            icon: <IconTools sx={iconSx} />,
            name: `${t('system.experimentalFeatures.mcp')} Tools`,
            description: t('system.experimentalFeatures.enableMCP'),
            path: '/mcp/sources',
            enabled: enableMCP,
            stage: 'exp' as const,
        },
        {
            key: 'guardrails',
            feature: 'guardrails',
            icon: <IconShield sx={iconSx} />,
            name: t('system.experimentalFeatures.guardrails'),
            description: t('system.experimentalFeatures.enableGuardrails'),
            path: '/guardrails',
            enabled: enableGuardrails,
            stage: 'exp' as const,
        },
        {
            key: 'skill_user',
            feature: 'skill_user' as const,
            icon: <IconSend sx={iconSx} />,
            name: t('system.experimentalFeatures.userPrompts'),
            description: t('system.experimentalFeatures.enableUserPrompts'),
            path: '/prompt/user',
            enabled: skillUser,
            stage: 'exp' as const,
        },
        {
            key: 'skill_ide',
            feature: 'skill_ide' as const,
            icon: <IconBolt sx={iconSx} />,
            name: t('system.experimentalFeatures.skills'),
            description: t('system.experimentalFeatures.enableIdeSkills'),
            path: '/prompt/skill',
            enabled: skillIde,
            stage: 'exp' as const,
        },
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
        // A non-modal Popper (not a Popover): a hover submenu needs the
        // pointer to reach it without a backdrop in between.
        <Popper
            open={Boolean(anchorEl)}
            anchorEl={anchorEl}
            placement="right-end"
            modifiers={[{ name: 'offset', options: { offset: [8, 4] } }]}
            sx={{ zIndex: Z_INDEX.popover + 1 }}
        >
            <Paper
                elevation={8}
                onMouseEnter={onMouseEnter}
                onMouseLeave={onMouseLeave}
                sx={{ width: 380, maxHeight: '80vh', overflowY: 'auto', borderRadius: 2 }}
            >
            <Box sx={{ px: 2, pt: 1.5, pb: 1 }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                    {t('scenarioOverview.powerUps.title')}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                    {t('scenarioOverview.powerUps.subtitle')}
                </Typography>
            </Box>
            <Divider />
            {failed && (
                <Alert severity="error" sx={{ m: 1.5 }}>
                    {t('system.experimentalFeatures.enableFailed')}
                </Alert>
            )}
            <Box sx={{ py: 0.5 }}>
                {powerUps.map((p) => (
                    <Stack key={p.key} direction="row" sx={{ alignItems: 'flex-start', pr: 1 }}>
                        <ListItemButton
                            disabled={!p.enabled}
                            onClick={() => { navigate(p.path); onClose(); onNavigate?.(); }}
                            // A disabled row still reads: only the click is off.
                            sx={{ flex: 1, minWidth: 0, py: 1, px: 2, alignItems: 'flex-start', gap: 1.5, '&.Mui-disabled': { opacity: 1 } }}
                        >
                            <Box sx={{ pt: 0.25, display: 'flex' }}>{p.icon}</Box>
                            <Box sx={{ minWidth: 0 }}>
                                <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center' }}>
                                    <Typography variant="body2" sx={{ fontWeight: 600, color: p.enabled ? 'text.primary' : 'text.secondary' }}>
                                        {p.name}
                                    </Typography>
                                    {p.stage && (
                                        <Tooltip
                                            title={p.stage === 'exp'
                                                ? t('scenarioOverview.powerUps.experimentalTooltip')
                                                : t('scenarioOverview.powerUps.betaTooltip')}
                                            arrow
                                        >
                                            <Chip
                                                size="small"
                                                color={p.stage === 'exp' ? 'warning' : 'info'}
                                                variant="outlined"
                                                label={p.stage === 'exp'
                                                    ? t('scenarioOverview.powerUps.experimental')
                                                    : t('scenarioOverview.powerUps.beta')}
                                                sx={{ height: 18, fontSize: '0.6875rem' }}
                                            />
                                        </Tooltip>
                                    )}
                                    {p.enabled && p.status && (
                                        <Typography variant="caption" sx={{ color: 'success.main' }}>{p.status}</Typography>
                                    )}
                                </Stack>
                                <Typography variant="caption" color="text.secondary" component="div" sx={{ lineHeight: 1.4 }}>
                                    {p.description}
                                </Typography>
                            </Box>
                        </ListItemButton>
                        {(p.feature || p.onToggle) && (
                            <Switch
                                size="small"
                                checked={p.enabled}
                                disabled={loading || updating !== undefined}
                                onChange={() => toggle(p)}
                                slotProps={{ input: { 'aria-label': p.name } }}
                                sx={{ mt: 1 }}
                            />
                        )}
                    </Stack>
                ))}
            </Box>
            {/* Consequence warnings for what is currently on. */}
            {powerUps.filter((p) => p.enabled && p.enabledNotice).map((p) => (
                <Alert key={p.key} severity="warning" sx={{ mx: 1.5, mb: 1.5 }}>
                    <strong>{p.name}</strong> — {p.enabledNotice}
                </Alert>
            ))}
            </Paper>
        </Popper>
    );
};
