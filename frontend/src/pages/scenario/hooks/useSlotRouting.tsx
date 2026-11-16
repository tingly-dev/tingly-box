import { useEffect, useState } from 'react';
import { Box, Button, Chip, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import ConfigRow from '@/components/ConfigRow';
import { api } from '@/services/api';
import { notify } from '@/utils/notify';

/** Claude Code's model slots and the env var each fills; default is the main rule. */
export const CLAUDE_CODE_SLOTS = [
    { slot: 'default', env: 'ANTHROPIC_MODEL' },
    { slot: 'haiku', env: 'ANTHROPIC_DEFAULT_HAIKU_MODEL' },
    { slot: 'sonnet', env: 'ANTHROPIC_DEFAULT_SONNET_MODEL' },
    { slot: 'opus', env: 'ANTHROPIC_DEFAULT_OPUS_MODEL' },
    { slot: 'fable', env: 'ANTHROPIC_DEFAULT_FABLE_MODEL' },
    { slot: 'subagent', env: 'CLAUDE_CODE_SUBAGENT_MODEL' },
] as const;

/** A slot's own rule (`builtin:<scenario>:<slot>`). */
export const slotRuleUuid = (scenario: string, slot: string) => `builtin:${scenario}:${slot}`;

const OWN_SLOTS = CLAUDE_CODE_SLOTS.filter(s => s.slot !== 'default').map(s => s.slot);

export interface SlotRouting {
    /** The default rule plus the rules of the slots that have one of their own. */
    rules: any[];
    setRules: (rules: any[]) => void;
    loading: boolean;
    /** The slots that have a rule of their own (the rest follow the default). */
    slots: string[];
    /** The Slots row: default always on, one switch per other slot, and split/merge all. */
    slotsRow: React.ReactNode;
    /** Re-read the rules and slots from the backend, e.g. after the default was switched to a profile. */
    reload: () => Promise<void>;
}

/**
 * Routing for clients with fixed model slots (Claude Code). A slot either
 * follows the default rule or has a rule of its own, switched from the Slots
 * row; the rules shown are the default rule plus the split slots' rules.
 * `enabled: false` keeps the hook inert for agents without slots (hooks can't
 * be called conditionally).
 */
export const useSlotRouting = (
    scenario: string,
    unifiedRuleUuid: string,
    enabled: boolean,
): SlotRouting => {
    const { t } = useTranslation();
    const [rules, setRules] = useState<any[]>([]);
    const [loading, setLoading] = useState(enabled);
    const [slots, setSlots] = useState<string[]>([]);
    const [busySlot, setBusySlot] = useState<string | null>(null);

    useEffect(() => {
        if (!enabled) return;
        let isMounted = true;
        setLoading(true);
        const load: Promise<[any[], string[]]> = Promise.all([api.getRules(scenario), api.getClaudeCodeSlots(scenario)]).then(([ruleResult, slotResult]) => {
            const all: any[] = ruleResult?.success ? ruleResult.data || [] : [];
            const split: string[] = slotResult?.success ? slotResult.data?.slots || [] : [];
            // The default rule first, then the rules of the split slots.
            const wanted = [unifiedRuleUuid, ...split.map(slot => slotRuleUuid(scenario, slot))];
            return [wanted.map(uuid => all.find(r => r.uuid === uuid)).filter(Boolean), split];
        });
        load.then(([next, split]) => {
            // A slower response for a previous scenario must not win.
            if (!isMounted) return;
            setRules(next);
            setSlots(split);
        }).catch((error) => {
            console.error('Failed to load Claude Code rules:', error);
        }).finally(() => {
            if (isMounted) setLoading(false);
        });
        return () => { isMounted = false; };
    }, [scenario, unifiedRuleUuid, enabled]);

    // Re-read rules and slots in place, without flipping `loading`.
    const reload = async () => {
        const [ruleResult, slotResult] = await Promise.all([api.getRules(scenario), api.getClaudeCodeSlots(scenario)]);
        const all: any[] = ruleResult?.success ? ruleResult.data || [] : [];
        const split: string[] = slotResult?.success ? slotResult.data?.slots || [] : [];
        setSlots(split);
        setRules([unifiedRuleUuid, ...split.map(slot => slotRuleUuid(scenario, slot))]
            .map(uuid => all.find(r => r.uuid === uuid)).filter(Boolean));
    };

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

    // Split every slot off the default, or fold them all back into it.
    const setAll = async (enable: boolean) => {
        setBusySlot('*');
        try {
            const missing = OWN_SLOTS.filter(slot => slots.includes(slot) !== enable);
            for (const slot of missing) {
                const result = await api.setClaudeCodeSlot(scenario, slot, enable);
                if (!result?.success) {
                    notify.show('error', `${t('claudeCode.slots.failed')}: ${result?.error || ''}`, { duration: 6000 });
                    break;
                }
            }
            await reload();
        } catch (error) {
            notify.show('error', `${t('claudeCode.slots.failed')}: ${String(error)}`, { duration: 6000 });
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
                        <Button size="small" sx={{ minWidth: 0, py: 0, textTransform: 'none' }} disabled={busySlot !== null}
                            onClick={() => void setAll(slots.length < OWN_SLOTS.length)}>
                            {t(slots.length < OWN_SLOTS.length ? 'claudeCode.slots.splitAll' : 'claudeCode.slots.mergeAll')}
                        </Button>
                    </Box>
                ),
            }]}
            activeTab="slots"
            onTabChange={() => {}}
        />
    ) : null;

    return { rules, setRules, loading, slots, slotsRow, reload };
};
