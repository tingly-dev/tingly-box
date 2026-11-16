import React, { useState } from 'react';
import { IconButton, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import DuplicateDialog from '@/components/DuplicateDialog';
import { Duplicate as DuplicateIcon } from '@/components/icons';
import { useProfileContext } from '@/contexts/ProfileContext';
import { MAIN_CONFIG_ID, useDuplicateProfile } from '@/pages/scenario/hooks/useDuplicateProfile';
import { suggestProfileCopyName } from '@/utils/copyName';

// "Duplicate as profile" on the main Claude Code page: a new profile with the
// main configuration's model rules and flags, which then lands on it.
const DuplicateMainProfileButton: React.FC = () => {
    const { t } = useTranslation();
    const { getProfiles } = useProfileContext();
    const duplicateProfile = useDuplicateProfile();
    const [open, setOpen] = useState(false);

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
                initialName={suggestProfileCopyName(MAIN_CONFIG_ID, getProfiles('claude_code').map(p => p.name))}
                onSubmit={(name) => duplicateProfile(MAIN_CONFIG_ID, name)}
            />
        </>
    );
};

export default DuplicateMainProfileButton;
