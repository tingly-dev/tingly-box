import React, { useState } from 'react';
import {
    Box,
    Button,
    ButtonGroup,
    Divider,
    ListItemIcon,
    MenuItem,
    Select,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import UnifiedCard from '@/components/UnifiedCard';
import { Add } from '@/components/icons';
import { api } from '@/services/api';
import { notify } from '@/utils/notify';
import { fontMono } from '@/theme/fonts';
import {
    CLAUDE_CODE_SLOT_ENV,
    type ClaudeCodeSlot,
    type ClaudeCodeSlotResolution,
    slotsUnified,
} from './claudeCodeSlots';

const FOLLOW = '__follow__';
const NEW_RULE = '__new__';

interface ClaudeCodeSlotsCardProps {
    /** claude_code or a profiled claude_code:<id>. */
    scenario: string;
    slots: ClaudeCodeSlotResolution[];
    /** Every rule of the scenario: the binding choices. */
    rules: any[];
    /** Called with the backend's new slot resolution after each change. */
    onChange: (slots: ClaudeCodeSlotResolution[]) => void;
    /** A profile's settings are rebuilt at launch; the main one must be re-applied. */
    isProfile?: boolean;
}

/**
 * The Claude Code slot table: for each model slot, what Claude Code sends and
 * which rule that is. Slots can share a rule or get their own, independently
 * — "everything unified, subagent on its own" is one change, not a mode.
 */
const ClaudeCodeSlotsCard: React.FC<ClaudeCodeSlotsCardProps> = ({ scenario, slots, rules, onChange, isProfile }) => {
    const { t } = useTranslation();
    const [busy, setBusy] = useState(false);

    const run = async (request: () => Promise<any>) => {
        setBusy(true);
        try {
            const result = await request();
            if (result?.success && result.data?.slots) {
                onChange(result.data.slots);
                notify.show('success', t(isProfile ? 'claudeCode.slots.updatedProfile' : 'claudeCode.slots.updated'), { duration: 6000 });
            } else {
                notify.show('error', `${t('claudeCode.slots.failed')}: ${result?.error || ''}`, { duration: 6000 });
            }
        } finally {
            setBusy(false);
        }
    };

    const handleSelect = (slot: ClaudeCodeSlot, value: string) => {
        if (value === NEW_RULE) {
            void run(() => api.createClaudeCodeSlotRule(scenario, slot));
        } else {
            void run(() => api.setClaudeCodeSlot(scenario, slot, value === FOLLOW ? '' : value));
        }
    };

    const unified = slotsUnified(slots);
    const ruleLabel = (rule: any) => (
        <Box sx={{ minWidth: 0 }}>
            <Typography variant="body2" noWrap sx={{ fontFamily: fontMono }}>{rule.request_model}</Typography>
            {rule.description && (
                <Typography variant="caption" noWrap sx={{ color: 'text.secondary', display: 'block' }}>
                    {rule.description}
                </Typography>
            )}
        </Box>
    );

    return (
        <UnifiedCard
            title={t('claudeCode.slots.title')}
            subtitle={t('claudeCode.slots.subtitle')}
            size="full"
            rightAction={
                <ButtonGroup size="small" variant="outlined" disabled={busy}>
                    <Button onClick={() => void run(() => api.applyClaudeCodeSlotPreset(scenario, 'unified'))} disabled={busy || unified}>
                        {t('claudeCode.slots.presetUnified')}
                    </Button>
                    <Button onClick={() => void run(() => api.applyClaudeCodeSlotPreset(scenario, 'separate'))} disabled={busy}>
                        {t('claudeCode.slots.presetSeparate')}
                    </Button>
                </ButtonGroup>
            }
        >
            <Box
                sx={{
                    display: 'grid',
                    gridTemplateColumns: { xs: '1fr', sm: 'minmax(140px, 1fr) minmax(160px, 1.2fr) minmax(220px, 1.6fr)' },
                    columnGap: 2,
                    rowGap: { xs: 0.5, sm: 1 },
                    alignItems: 'center',
                }}
            >
                {[t('claudeCode.slots.slot'), t('claudeCode.slots.sends'), t('claudeCode.slots.rule')].map(h => (
                    <Typography key={h} variant="caption" sx={{ color: 'text.secondary', display: { xs: 'none', sm: 'block' } }}>
                        {h}
                    </Typography>
                ))}
                {slots.map(slot => {
                    const follows = slot.slot !== 'default' && !slot.bound;
                    const value = follows ? FOLLOW : slot.rule_uuid;
                    return (
                        <React.Fragment key={slot.slot}>
                            <Box sx={{ minWidth: 0, pt: { xs: 1, sm: 0 } }}>
                                <Typography variant="body2" sx={{ fontWeight: 600, textTransform: 'capitalize' }}>{slot.slot}</Typography>
                                <Typography variant="caption" noWrap sx={{ color: 'text.disabled', fontFamily: fontMono, display: 'block' }}>
                                    {CLAUDE_CODE_SLOT_ENV[slot.slot]}
                                </Typography>
                            </Box>
                            <Typography
                                variant="body2"
                                noWrap
                                sx={{ fontFamily: fontMono, color: follows ? 'text.secondary' : 'text.primary' }}
                            >
                                {slot.request_model}{slot.context_1m ? '[1m]' : ''}
                            </Typography>
                            <Select
                                size="small"
                                fullWidth
                                value={value || ''}
                                disabled={busy}
                                onChange={e => handleSelect(slot.slot, e.target.value as string)}
                                renderValue={v => (v === FOLLOW
                                    ? <Typography variant="body2" sx={{ color: 'text.secondary' }}>{t('claudeCode.slots.followDefault')}</Typography>
                                    : <Typography variant="body2" noWrap sx={{ fontFamily: fontMono }}>
                                        {rules.find(r => r.uuid === v)?.request_model ?? slot.request_model}
                                    </Typography>)}
                            >
                                {slot.slot !== 'default' && (
                                    <MenuItem value={FOLLOW}>{t('claudeCode.slots.followDefault')}</MenuItem>
                                )}
                                {slot.slot !== 'default' && <Divider />}
                                {rules.map(rule => (
                                    <MenuItem key={rule.uuid} value={rule.uuid}>{ruleLabel(rule)}</MenuItem>
                                ))}
                                <Divider />
                                <MenuItem value={NEW_RULE}>
                                    <ListItemIcon><Add fontSize="small" /></ListItemIcon>
                                    {t('claudeCode.slots.newRule')}
                                </MenuItem>
                            </Select>
                        </React.Fragment>
                    );
                })}
            </Box>
        </UnifiedCard>
    );
};

export default ClaudeCodeSlotsCard;
