import { useCallback, useEffect, useState } from 'react';
import { Box, Chip, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { ConfigRow } from '@/components/ConfigRow';
import { api } from '@/services/api';
import { notify } from '@/utils/notify';

/** Claude Code's model slots and the env var each one fills. */
export const CLAUDE_CODE_SLOTS = [
    { slot: 'default', env: 'ANTHROPIC_MODEL' },
    { slot: 'haiku', env: 'ANTHROPIC_DEFAULT_HAIKU_MODEL' },
    { slot: 'sonnet', env: 'ANTHROPIC_DEFAULT_SONNET_MODEL' },
    { slot: 'opus', env: 'ANTHROPIC_DEFAULT_OPUS_MODEL' },
    { slot: 'fable', env: 'ANTHROPIC_DEFAULT_FABLE_MODEL' },
    { slot: 'subagent', env: 'CLAUDE_CODE_SUBAGENT_MODEL' },
] as const;

/** A slot's own rule (`builtin:<scenario>:<slot>`); "cc" is the main rule. */
export const slotRuleUuid = (scenario: string, slot: string) => `builtin:${scenario}:${slot}`;

export interface SlotRouting {
    /** The scenario's active rules, main rule first. */
    rules: any[];
    setRules: (rules: any[]) => void;
    loading: boolean;
    /** The Slots row: default (always on), then one switchable chip per slot. */
    slotsRow: React.ReactNode;
}

/**
 * Routing for clients with fixed model slots (Claude Code). The default slot
 * is the main rule and always there; every other slot follows it unless it
 * has an active rule of its own, which its chip adds or removes. All on is what used to be "separate", none
 * "unified". `enabled: false` keeps the hook inert for agents without slots
 * (hooks can't be called conditionally).
 */
export const useSlotRouting = (scenario: string, enabled: boolean): SlotRouting => {
    const { t } = useTranslation();
    const [allRules, setAllRules] = useState<any[]>([]);
    const [loading, setLoading] = useState(enabled);
    const [busySlot, setBusySlot] = useState<string | null>(null);

    const load = useCallback(async () => {
        const result = await api.getRules(scenario);
        setAllRules(result?.success ? result.data || [] : []);
    }, [scenario]);

    useEffect(() => {
        if (!enabled) return;
        setLoading(true);
        void load().finally(() => setLoading(false));
    }, [enabled, load]);

    // The main rule leads.
    const mainUuid = slotRuleUuid(scenario, 'cc');
    const rules = allRules
        .filter(r => r.active)
        .sort((a, b) => Number(b.uuid === mainUuid) - Number(a.uuid === mainUuid));

    // The rules card edits the active subset; merge its changes back.
    const setRules = useCallback((next: any[]) => {
        setAllRules(prev => prev.map(r => next.find(n => n.uuid === r.uuid) ?? r));
    }, []);

    const hasOwnRule = (slot: string) => rules.some(r => r.uuid === slotRuleUuid(scenario, slot));

    const toggle = async (slot: string) => {
        setBusySlot(slot);
        try {
            const result = await api.setClaudeCodeSlot(scenario, slot, !hasOwnRule(slot));
            if (result?.success) {
                await load();
            } else {
                notify.show('error', `${t('claudeCode.slots.failed')}: ${result?.error || ''}`, { duration: 6000 });
            }
        } finally {
            setBusySlot(null);
        }
    };

    const slotsRow = enabled ? (
        <ConfigRow
            tabs={[{
                key: 'slots',
                label: t('claudeCode.slots.label'),
                content: (
                    <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.75 }}>
                        {CLAUDE_CODE_SLOTS.map(({ slot, env }) => {
                            // The default slot is the main rule: always there, not a switch.
                            const isDefault = slot === 'default';
                            const on = isDefault || hasOwnRule(slot);
                            const hint = isDefault ? 'claudeCode.slots.defaultRule' : on ? 'claudeCode.slots.ownRule' : 'claudeCode.slots.mainRule';
                            return (
                                <Tooltip key={slot} arrow title={`${env} — ${t(hint)}`}>
                                    <Chip
                                        size="small"
                                        label={slot}
                                        color={on ? 'primary' : 'default'}
                                        variant={on ? 'filled' : 'outlined'}
                                        disabled={!isDefault && busySlot !== null}
                                        onClick={isDefault ? undefined : () => void toggle(slot)}
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

    return { rules, setRules, loading, slotsRow };
};
