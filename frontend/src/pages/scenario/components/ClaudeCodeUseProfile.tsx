import React, { useState } from 'react';
import { Box, Chip, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import ConfigRow from '@/components/ConfigRow';
import ConfirmDialog from '@/components/ConfirmDialog';
import { useProfileContext } from '@/contexts/ProfileContext';
import { api } from '@/services/api';
import { notify } from '@/utils/notify';

interface Props {
    /** Called after the default's routing changed, so the page re-reads its rules. */
    onChanged: () => Promise<void> | void;
}

/**
 * "Use profile" row of the default Claude Code config: one chip per profile;
 * picking one (after a confirm) overwrites the default's routing with a copy of
 * that profile's. The profile is never changed, and the default's previous
 * routing is not kept — the dialog says so and points at "Duplicate as profile".
 * Only the default page renders this; profiles have no such row.
 */
const ClaudeCodeUseProfile: React.FC<Props> = ({ onChanged }) => {
    const { t } = useTranslation();
    const { getProfiles } = useProfileContext();
    const profiles = getProfiles('claude_code');
    const [target, setTarget] = useState<{ id: string; name: string } | null>(null);
    const [busy, setBusy] = useState(false);

    if (profiles.length === 0) return null;

    const confirm = async () => {
        if (!target) return;
        setBusy(true);
        try {
            const result = await api.copyProfileToClaudeCodeDefault(target.id);
            if (!result?.success) {
                notify.show('error', `${t('claudeCode.useProfile.failed')}: ${result?.error || ''}`, { duration: 6000 });
                return;
            }
            await onChanged();
            notify.show('success', t('claudeCode.useProfile.done', { name: target.name }));
            setTarget(null);
        } catch (error) {
            notify.show('error', `${t('claudeCode.useProfile.failed')}: ${String(error)}`, { duration: 6000 });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Box sx={{ px: 2, pt: 1 }}>
            <ConfigRow
                activeTab="use-profile"
                onTabChange={() => undefined}
                tabs={[{
                    key: 'use-profile',
                    label: t('claudeCode.useProfile.label'),
                    content: (
                        <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 0.75 }}>
                            {profiles.map(profile => (
                                <Tooltip key={profile.id} arrow title={t('claudeCode.useProfile.chipHint')}>
                                    <Chip
                                        size="small"
                                        label={profile.name}
                                        variant="outlined"
                                        onClick={() => setTarget({ id: profile.id, name: profile.name })}
                                    />
                                </Tooltip>
                            ))}
                        </Box>
                    ),
                }]}
            />
            <ConfirmDialog
                open={!!target}
                title={t('claudeCode.useProfile.dialogTitle', { name: target?.name })}
                description={(
                    <>
                        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                            {t('claudeCode.useProfile.dialogBody', { name: target?.name })}
                        </Typography>
                        <Typography variant="body2" color="warning.main">
                            {t('claudeCode.useProfile.dialogWarning')}
                        </Typography>
                    </>
                )}
                confirmLabel={t('claudeCode.useProfile.confirm')}
                cancelLabel={t('common.cancel')}
                loading={busy}
                onClose={() => setTarget(null)}
                onConfirm={() => void confirm()}
            />
        </Box>
    );
};

export default ClaudeCodeUseProfile;
