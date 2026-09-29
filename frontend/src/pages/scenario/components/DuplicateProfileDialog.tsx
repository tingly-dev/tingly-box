import React from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import DuplicateDialog from '@/components/DuplicateDialog';
import { useProfileContext } from '@/contexts/ProfileContext';
import { nextFreeName } from '@/utils/duplicateName';

// Duplicates a Claude Code profile — or the main configuration, with
// sourceId 'default' — into a new profile, then lands on it.
export const DuplicateProfileDialog: React.FC<{ open: boolean; onClose: () => void; sourceId: string; sourceName: string }> = ({
    open, onClose, sourceId, sourceName,
}) => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { getProfiles, refresh } = useProfileContext();
    return (
        <DuplicateDialog
            open={open}
            onClose={onClose}
            scenario="claude_code"
            sourceId={sourceId}
            defaultName={nextFreeName(sourceName, getProfiles('claude_code').map((p) => p.name), '-')}
            title={t('claudeCode.profile.duplicateProfile')}
            hint={t(sourceId === 'default' ? 'claudeCode.profile.duplicateMainHint' : 'claudeCode.profile.duplicateHint')}
            nameLabel={t('claudeCode.profile.profileName')}
            onDuplicated={async ({ id }) => {
                await refresh();
                navigate(`/agent/claude_code/profile/${id}`);
            }}
        />
    );
};

export default DuplicateProfileDialog;
