import React, { useCallback, useEffect, useState } from 'react';
import { Box, Chip, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import ConfigRow from '@/components/ConfigRow';
import { useProfileContext } from '@/contexts/ProfileContext';
import { api } from '@/services/api';
import { notify } from '@/utils/notify';

interface DefaultSwitchStatus {
    switched: boolean;
    profile_id?: string;
    profile_name?: string;
    profile_exists: boolean;
    in_sync: boolean;
}

interface Props {
    /** Called after the default's routing changed, so the page re-reads its rules. */
    onChanged: () => Promise<void> | void;
}

/**
 * "Switch" row of the default Claude Code config: what the default routes like.
 * One chip for its own routing and one per profile; the lit chip is the current
 * one, any other chip copies that profile's routing into the default (the profile
 * itself is never changed), and the own chip restores what the default was before
 * the first switch. Only the default page renders this — profiles have no such row.
 */
const ClaudeCodeDefaultProfileSwitch: React.FC<Props> = ({ onChanged }) => {
    const { t } = useTranslation();
    const { getProfiles } = useProfileContext();
    const profiles = getProfiles('claude_code');
    const [status, setStatus] = useState<DefaultSwitchStatus | null>(null);
    const [busy, setBusy] = useState(false);

    const load = useCallback(async () => {
        try {
            const result = await api.getClaudeCodeDefaultSwitch();
            if (result?.success) setStatus(result.data);
        } catch (error) {
            console.error('Failed to load the default switch state:', error);
        }
    }, []);

    useEffect(() => { void load(); }, [load]);

    // Nothing to switch to, and nothing to restore.
    if (profiles.length === 0 && !status?.switched) return null;

    const run = async (call: () => Promise<any>, successKey: string, name?: string) => {
        setBusy(true);
        try {
            const result = await call();
            if (!result?.success) {
                notify.show('error', `${t('claudeCode.defaultSwitch.failed')}: ${result?.error || ''}`, { duration: 6000 });
                return;
            }
            setStatus(result.data);
            await onChanged();
            notify.show('success', t(successKey, { name }));
        } catch (error) {
            notify.show('error', `${t('claudeCode.defaultSwitch.failed')}: ${String(error)}`, { duration: 6000 });
        } finally {
            setBusy(false);
        }
    };

    const switched = !!status?.switched;
    const ownOn = !switched;
    // The profile the default was copied from is gone: no chip is lit, say why.
    const orphan = switched && !status?.profile_exists;

    return (
        <Box sx={{ px: 2, pt: 1 }}>
            <ConfigRow
                activeTab="switch"
                onTabChange={() => undefined}
                tabs={[{
                    key: 'switch',
                    label: t('claudeCode.defaultSwitch.label'),
                    content: (
                        <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 0.75 }}>
                            <Typography variant="body2" color="text.secondary" sx={{ mr: 0.5 }}>
                                {t('claudeCode.defaultSwitch.lead')}
                            </Typography>
                            <Tooltip arrow title={t(ownOn ? 'claudeCode.defaultSwitch.ownOn' : 'claudeCode.defaultSwitch.ownOff')}>
                                <Chip
                                    size="small"
                                    label={t('claudeCode.defaultSwitch.own')}
                                    color={ownOn ? 'primary' : 'default'}
                                    variant={ownOn ? 'filled' : 'outlined'}
                                    disabled={busy}
                                    onClick={ownOn ? undefined : () => void run(() => api.restoreClaudeCodeDefault(), 'claudeCode.defaultSwitch.restored')}
                                />
                            </Tooltip>
                            {profiles.map(profile => {
                                const current = switched && status?.profile_id === profile.id;
                                const edited = current && !status?.in_sync;
                                const clickable = !current || edited;
                                return (
                                    <Tooltip
                                        key={profile.id}
                                        arrow
                                        title={t(edited ? 'claudeCode.defaultSwitch.profileEdited' : current ? 'claudeCode.defaultSwitch.profileOn' : 'claudeCode.defaultSwitch.profileOff')}
                                    >
                                        <Chip
                                            size="small"
                                            label={edited ? t('claudeCode.defaultSwitch.editedLabel', { name: profile.name }) : profile.name}
                                            color={current ? 'primary' : 'default'}
                                            variant={current ? 'filled' : 'outlined'}
                                            disabled={busy}
                                            onClick={clickable ? () => void run(() => api.switchClaudeCodeDefaultToProfile(profile.id), 'claudeCode.defaultSwitch.applied', profile.name) : undefined}
                                        />
                                    </Tooltip>
                                );
                            })}
                            {orphan && (
                                <Typography variant="caption" color="text.secondary">
                                    {t('claudeCode.defaultSwitch.profileDeleted', { name: status?.profile_name })}
                                </Typography>
                            )}
                        </Box>
                    ),
                }]}
            />
        </Box>
    );
};

export default ClaudeCodeDefaultProfileSwitch;
