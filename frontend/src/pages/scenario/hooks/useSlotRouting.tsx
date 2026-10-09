import { useEffect, useState } from 'react';
import {
    Box,
    Button,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    ToggleButton,
    ToggleButtonGroup,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { api } from '@/services/api';
import { ConfigRow } from '@/components/ConfigRow';
import { toggleButtonGroupStyle, toggleButtonStyle } from '@/styles/toggleStyles';
import { notify } from '@/utils/notify';

/**
 * How a fixed-slot client's model slots map to rules:
 * - `unified`: every slot goes through one built-in rule;
 * - `separate`: each slot has its own rule.
 */
export type SlotMode = 'unified' | 'separate';

/** A slot's own rule (`builtin:<scenario>:<slot>`). */
export const slotRuleUuid = (scenario: string, slot: string) => `builtin:${scenario}:${slot}`;

/** Claude Code's model slots and the env var each fills; default is the main rule. */
export const CLAUDE_CODE_SLOTS = [
    { slot: 'default', env: 'ANTHROPIC_MODEL' },
    { slot: 'haiku', env: 'ANTHROPIC_DEFAULT_HAIKU_MODEL' },
    { slot: 'sonnet', env: 'ANTHROPIC_DEFAULT_SONNET_MODEL' },
    { slot: 'opus', env: 'ANTHROPIC_DEFAULT_OPUS_MODEL' },
    { slot: 'fable', env: 'ANTHROPIC_DEFAULT_FABLE_MODEL' },
    { slot: 'subagent', env: 'CLAUDE_CODE_SUBAGENT_MODEL' },
] as const;

export interface SlotRouting {
    mode: SlotMode;
    /** The rules for the current mode. */
    rules: any[];
    setRules: (rules: any[]) => void;
    loading: boolean;
    /** Unified / Separate switch; changing it asks for confirmation first. */
    modeSwitch: React.ReactNode;
    /** The confirmation dialog for a mode change. */
    modeDialog: React.ReactNode;
    /** Unified mode: the slots that have a rule of their own. */
    slots: string[];
    /** Unified mode: the Slots row (default always on, one switch per other slot). */
    slotsRow: React.ReactNode;
}

/**
 * Routing for clients with fixed model slots (Claude Code): the mode lives in
 * the scenario's flags (or, for a profile, is given as `fixedMode`), and the
 * rules shown depend on it. Separate mode shows every rule but the unified
 * one. Unified mode shows the unified rule plus the rules of the slots that
 * have one of their own, switched from the Slots row. `enabled: false` keeps
 * the hook inert for agents without slots (hooks can't be called
 * conditionally).
 */
export const useSlotRouting = (
    scenario: string,
    unifiedRuleUuid: string,
    enabled: boolean,
    fixedMode?: SlotMode,
): SlotRouting => {
    const { t } = useTranslation();
    const modes: { value: SlotMode; label: string; description: string }[] = [
        { value: 'unified', label: t('claudeCode.configModes.unified.label'), description: t('claudeCode.configModes.unified.description') },
        { value: 'separate', label: t('claudeCode.configModes.separate.label'), description: t('claudeCode.configModes.separate.description') },
    ];
    const modeLabel = (mode: SlotMode | null) => modes.find(m => m.value === mode)?.label ?? mode ?? '';

    const [mode, setMode] = useState<SlotMode>(fixedMode ?? 'unified');
    const [pendingMode, setPendingMode] = useState<SlotMode | null>(null);
    // Separate from pendingMode so the dialog's text stays put while it fades out.
    const [dialogOpen, setDialogOpen] = useState(false);
    const [rules, setRules] = useState<any[]>([]);
    const [loading, setLoading] = useState(enabled);
    const [slots, setSlots] = useState<string[]>([]);
    const [busySlot, setBusySlot] = useState<string | null>(null);

    useEffect(() => {
        if (fixedMode) setMode(fixedMode);
    }, [fixedMode]);

    useEffect(() => {
        if (!enabled || fixedMode) return;
        api.getScenarioConfig(scenario).then((result) => {
            if (result.success && result.data && result.data.flags) {
                setMode(result.data.flags.separate ? 'separate' : 'unified');
            }
        }).catch((error) => {
            console.error('Failed to load scenario config:', error);
        });
    }, [scenario, enabled, fixedMode]);

    useEffect(() => {
        if (!enabled) return;
        let isMounted = true;
        setLoading(true);
        const load: Promise<[any[], string[]]> = mode === 'unified'
            ? Promise.all([api.getRules(scenario), api.getClaudeCodeSlots(scenario)]).then(([ruleResult, slotResult]) => {
                const all: any[] = ruleResult?.success ? ruleResult.data || [] : [];
                const split: string[] = slotResult?.success ? slotResult.data?.slots || [] : [];
                // The unified rule first, then the rules of the split slots.
                const wanted = [unifiedRuleUuid, ...split.map(slot => slotRuleUuid(scenario, slot))];
                const shown = wanted.map(uuid => all.find(r => r.uuid === uuid)).filter(Boolean);
                return [shown, split];
            })
            // Separate mode shows every rule but the unified one.
            : api.getRules(scenario).then((result) =>
                [(result.success ? result.data : []).filter((r: any) => r.uuid !== unifiedRuleUuid), []]);
        load.then(([next, split]) => {
            // A slower response for a previous scenario or mode must not win.
            if (!isMounted) return;
            setRules(next);
            setSlots(split);
        }).catch((error) => {
            console.error('Failed to load Claude Code rules:', error);
        }).finally(() => {
            if (isMounted) setLoading(false);
        });
        return () => { isMounted = false; };
    }, [scenario, unifiedRuleUuid, mode, enabled]);

    // Apply a slot switch from the response in place: a reload would flip
    // `loading` and blank the page.
    const toggleSlot = async (slot: string) => {
        const enable = !slots.includes(slot);
        setBusySlot(slot);
        try {
            const result = await api.setClaudeCodeSlot(scenario, slot, enable);
            if (!result?.success) {
                notify.show('error', `${t('claudeCode.slots.failed')}: ${result?.error || ''}`, { duration: 6000 });
                return;
            }
            const split: string[] = result.data?.slots ?? [];
            const rule = result.data?.rule;
            setSlots(split);
            setRules(prev => {
                const rest = prev.filter(r => r.uuid !== slotRuleUuid(scenario, slot));
                if (!enable || !rule?.uuid) return rest;
                // Keep slot order: the unified rule, then the split slots.
                const order = [unifiedRuleUuid, ...split.map(s => slotRuleUuid(scenario, s))];
                return [...rest, rule].sort((a, b) => order.indexOf(a.uuid) - order.indexOf(b.uuid));
            });
        } catch (error) {
            notify.show('error', `${t('claudeCode.slots.failed')}: ${String(error)}`, { duration: 6000 });
        } finally {
            setBusySlot(null);
        }
    };

    const slotsRow = enabled && mode === 'unified' ? (
        <ConfigRow
            tabs={[{
                key: 'slots',
                label: t('claudeCode.slots.label'),
                content: (
                    <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.75 }}>
                        {CLAUDE_CODE_SLOTS.map(({ slot, env }) => {
                            // The default slot is the unified rule itself: always on, not a switch.
                            const isDefault = slot === 'default';
                            const on = isDefault || slots.includes(slot);
                            const hint = isDefault ? 'claudeCode.slots.defaultRule' : on ? 'claudeCode.slots.ownRule' : 'claudeCode.slots.mainRule';
                            return (
                                <Tooltip key={slot} arrow title={`${env} — ${t(hint)}`}>
                                    <Chip
                                        size="small"
                                        label={slot}
                                        color={on ? 'primary' : 'default'}
                                        variant={on ? 'filled' : 'outlined'}
                                        disabled={!isDefault && busySlot !== null}
                                        onClick={isDefault ? undefined : () => void toggleSlot(slot)}
                                    />
                                </Tooltip>
                            );
                        })}
                    </Box>
                ),
            }]}
            activeTab="slots"
            onTabChange={() => {}}
        />
    ) : null;

    const confirmModeChange = async () => {
        if (!pendingMode) return;
        const next = pendingMode;
        setDialogOpen(false);
        try {
            // GET-merge: SetScenarioConfig replaces the record wholesale,
            // so a partial payload silently wipes extensions (e.g. vision_proxy_service).
            const current = (await api.getScenarioConfig(scenario))?.data || {};
            const result = await api.setScenarioConfig(scenario, {
                ...current,
                scenario,
                flags: {
                    ...(current.flags || {}),
                    unified: next === 'unified',
                    separate: next === 'separate',
                    smart: false,
                },
            });
            if (result.success) {
                setMode(next);
                notify.show('success', t('claudeCode.modeChange.success', { mode: modeLabel(next) }), { duration: 6000 });
            } else {
                notify.show('error', t('claudeCode.modeChange.failed'), { duration: 6000 });
            }
        } catch (error) {
            console.error('Failed to save scenario config:', error);
            notify.show('error', t('claudeCode.modeChange.failed'), { duration: 6000 });
        } finally {
            setPendingMode(null);
        }
    };

    const cancelModeChange = () => {
        setDialogOpen(false);
        setPendingMode(null);
    };

    const modeSwitch = fixedMode ? null : (
        <ToggleButtonGroup
            value={mode}
            exclusive
            size="small"
            onChange={(_, value: SlotMode | null) => {
                if (!value || value === mode) return;
                setPendingMode(value);
                setDialogOpen(true);
            }}
            sx={toggleButtonGroupStyle}
        >
            {modes.map((m) => (
                <Tooltip key={m.value} title={m.description} arrow>
                    <ToggleButton value={m.value} sx={toggleButtonStyle}>
                        {m.label}
                    </ToggleButton>
                </Tooltip>
            ))}
        </ToggleButtonGroup>
    );

    const modeDialog = (
        <Dialog open={dialogOpen} onClose={cancelModeChange} maxWidth="sm" fullWidth>
            <DialogTitle>{t('claudeCode.modeChange.title')}</DialogTitle>
            <DialogContent>
                <Typography variant="body1" sx={{ mb: 1 }}>
                    {t('claudeCode.modeChange.body', { from: modeLabel(mode), to: modeLabel(pendingMode) })}
                </Typography>
                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                    {t('claudeCode.modeChange.hint')}
                </Typography>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2, gap: 1, justifyContent: 'flex-end' }}>
                <Button onClick={cancelModeChange} color="inherit" size="small">
                    {t('claudeCode.modeChange.cancel')}
                </Button>
                <Button onClick={confirmModeChange} variant="contained" size="small">
                    {t('claudeCode.modeChange.confirm')}
                </Button>
            </DialogActions>
        </Dialog>
    );

    return { mode, rules, setRules, loading, modeSwitch, modeDialog, slots, slotsRow };
};
