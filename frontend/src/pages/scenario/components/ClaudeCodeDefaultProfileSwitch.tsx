import React, { useCallback, useEffect, useState } from 'react';
import { Box, Button, Divider, ListItemText, Menu, MenuItem, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import ConfigRow from '@/components/ConfigRow';
import { Check, ExpandMore } from '@/components/icons';
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
 * "Apply profile…" on the default Claude Code config: the default takes a copy
 * of a profile's routing in one pick, the profile stays as it is, and
 * "Restore my default" brings back what the default was before the first switch.
 * Only the default page renders this — profiles have no such control.
 */
const ClaudeCodeDefaultProfileSwitch: React.FC<Props> = ({ onChanged }) => {
    const { t } = useTranslation();
    const { getProfiles } = useProfileContext();
    const profiles = getProfiles('claude_code');
    const [status, setStatus] = useState<DefaultSwitchStatus | null>(null);
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
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
        setAnchor(null);
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

    const summary = !status?.switched
        ? t('claudeCode.defaultSwitch.own')
        : !status.profile_exists
            ? t('claudeCode.defaultSwitch.profileDeleted', { name: status.profile_name })
            : status.in_sync
                ? t('claudeCode.defaultSwitch.mirrors', { name: status.profile_name })
                : t('claudeCode.defaultSwitch.edited', { name: status.profile_name });

    return (
        <Box sx={{ px: 2, pt: 1 }}>
            <ConfigRow
                activeTab="profile"
                onTabChange={() => undefined}
                tabs={[{
                    key: 'profile',
                    label: t('claudeCode.defaultSwitch.label'),
                    content: (
                        <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 1 }}>
                            <Typography variant="body2" color="text.secondary">{summary}</Typography>
                            <Button
                                size="small"
                                disabled={busy}
                                endIcon={<ExpandMore fontSize="small" />}
                                sx={{ textTransform: 'none', py: 0 }}
                                onClick={(e) => setAnchor(e.currentTarget)}
                            >
                                {t('claudeCode.defaultSwitch.apply')}
                            </Button>
                            <Menu anchorEl={anchor} open={!!anchor} onClose={() => setAnchor(null)}>
                                {profiles.map(profile => (
                                    <MenuItem
                                        key={profile.id}
                                        selected={status?.switched && status.profile_id === profile.id}
                                        onClick={() => void run(() => api.switchClaudeCodeDefaultToProfile(profile.id), 'claudeCode.defaultSwitch.applied', profile.name)}
                                    >
                                        <ListItemText primary={profile.name} />
                                        {status?.switched && status.profile_id === profile.id && status.in_sync && <Check fontSize="small" sx={{ ml: 2 }} />}
                                    </MenuItem>
                                ))}
                                {status?.switched && <Divider />}
                                {status?.switched && (
                                    <MenuItem onClick={() => void run(() => api.restoreClaudeCodeDefault(), 'claudeCode.defaultSwitch.restored')}>
                                        <ListItemText primary={t('claudeCode.defaultSwitch.restore')} secondary={t('claudeCode.defaultSwitch.restoreHint')} />
                                    </MenuItem>
                                )}
                            </Menu>
                        </Box>
                    ),
                }]}
            />
        </Box>
    );
};

export default ClaudeCodeDefaultProfileSwitch;
