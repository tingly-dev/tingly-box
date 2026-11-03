import React, { useState } from 'react';
import { IconButton, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import DuplicateDialog from '@/components/DuplicateDialog';
import { Duplicate as DuplicateIcon } from '@/components/icons';
import { useProfileContext } from '@/contexts/ProfileContext';
import { api } from '@/services/api';
import { useNotify } from '@/hooks/useNotify';
import { suggestProfileCopyName } from '@/utils/copyName';

// "Duplicate as profile" on the main Claude Code page: a new profile with the
// main configuration's model rules and flags, which then lands on it.
const DuplicateMainProfileButton: React.FC = () => {
    const { t } = useTranslation();
    const notify = useNotify();
    const navigate = useNavigate();
    const { getProfiles, refresh } = useProfileContext();
    const [open, setOpen] = useState(false);

    const submit = async (name: string): Promise<boolean> => {
        try {
            const result = await api.duplicateProfile('claude_code', 'default', name);
            if (result.success) {
                notify.success(t('claudeCode.profile.profileDuplicated'));
                refresh();
                navigate(`/agent/claude_code/profile/${result.data.id}`);
                return true;
            }
            notify.error(`${t('claudeCode.profile.duplicateFailed')}: ${result.error || 'Unknown error'}`);
        } catch {
            notify.error(t('claudeCode.profile.duplicateFailed'));
        }
        return false;
    };

    return (
        <>
            <Tooltip title={t('claudeCode.profile.duplicateMain')}>
                <IconButton size="small" aria-label={t('claudeCode.profile.duplicateMain')} onClick={() => setOpen(true)}>
                    <DuplicateIcon fontSize="small" sx={{ color: 'text.secondary' }} />
                </IconButton>
            </Tooltip>
            <DuplicateDialog
                open={open}
                onClose={() => setOpen(false)}
                title={t('claudeCode.profile.duplicateMainTitle')}
                description={t('claudeCode.profile.duplicateMainDescription')}
                label={t('claudeCode.profile.profileName')}
                initialName={suggestProfileCopyName('default', getProfiles('claude_code').map(p => p.name))}
                onSubmit={submit}
            />
        </>
    );
};

export default DuplicateMainProfileButton;
