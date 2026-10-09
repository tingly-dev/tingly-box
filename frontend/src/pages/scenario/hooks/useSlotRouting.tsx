import { useEffect, useState } from 'react';
import {
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { api } from '@/services/api';
import { notify } from '@/utils/notify';

/**
 * How a fixed-slot client's model slots map to rules:
 * - `unified`: every slot goes through one built-in rule;
 * - `separate`: each slot has its own rule.
 */
export type SlotMode = 'unified' | 'separate';

export interface SlotRouting {
    mode: SlotMode;
    /** The rules for the current mode. */
    rules: any[];
    setRules: (rules: any[]) => void;
    loading: boolean;
    /** Unified / Separate, as options for a choice control. */
    modeOptions: { value: SlotMode; label: string; tooltip: string }[];
    /** Ask to change the mode; the user confirms in `modeDialog` first. */
    requestMode: (mode: SlotMode) => void;
    /** The confirmation dialog for a mode change. */
    modeDialog: React.ReactNode;
}

/**
 * Routing for clients with fixed model slots (Claude Code): the mode lives in
 * the scenario's flags, and the rules shown depend on it — the one unified
 * rule, or every other rule of the scenario. `enabled: false` keeps the hook
 * inert for agents without slots (hooks can't be called conditionally).
 */
export const useSlotRouting = (scenario: string, unifiedRuleUuid: string, enabled: boolean): SlotRouting => {
    const { t } = useTranslation();
    const modes: { value: SlotMode; label: string; description: string }[] = [
        { value: 'unified', label: t('claudeCode.configModes.unified.label'), description: t('claudeCode.configModes.unified.description') },
        { value: 'separate', label: t('claudeCode.configModes.separate.label'), description: t('claudeCode.configModes.separate.description') },
    ];
    const modeLabel = (mode: SlotMode | null) => modes.find(m => m.value === mode)?.label ?? mode ?? '';

    const [mode, setMode] = useState<SlotMode>('unified');
    const [pendingMode, setPendingMode] = useState<SlotMode | null>(null);
    // Separate from pendingMode so the dialog's text stays put while it fades out.
    const [dialogOpen, setDialogOpen] = useState(false);
    const [rules, setRules] = useState<any[]>([]);
    const [loading, setLoading] = useState(enabled);

    useEffect(() => {
        if (!enabled) return;
        api.getScenarioConfig(scenario).then((result) => {
            if (result.success && result.data && result.data.flags) {
                setMode(result.data.flags.separate ? 'separate' : 'unified');
            }
        }).catch((error) => {
            console.error('Failed to load scenario config:', error);
        });
    }, [scenario, enabled]);

    useEffect(() => {
        if (!enabled) return;
        let isMounted = true;
        setLoading(true);
        const load = mode === 'unified'
            ? api.getRule(unifiedRuleUuid).then((result) => (result.success ? [result.data] : []))
            // Separate mode shows every rule but the unified one.
            : api.getRules(scenario).then((result) =>
                (result.success ? result.data : []).filter((r: any) => r.uuid !== unifiedRuleUuid));
        load.then((next) => {
            if (!isMounted) return;
            setRules(next);
            setLoading(false);
        });
        return () => { isMounted = false; };
    }, [scenario, unifiedRuleUuid, mode, enabled]);

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

    // Unified / Separate as plain options + a request function, so the page can lay
    // the choice out like its other rows; a change still asks for confirmation.
    const modeOptions = modes.map((m) => ({ value: m.value, label: m.label, tooltip: m.description }));
    const requestMode = (value: SlotMode) => {
        if (value === mode) return;
        setPendingMode(value);
        setDialogOpen(true);
    };

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

    return { mode, rules, setRules, loading, modeOptions, requestMode, modeDialog };
};
