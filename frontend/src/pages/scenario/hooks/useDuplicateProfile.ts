import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useProfileContext } from '@/contexts/ProfileContext';
import { useNotify } from '@/hooks/useNotify';
import { api } from '@/services/api';

const BASE_SCENARIO = 'claude_code';

/** The main Claude Code configuration, where a profile id is expected. */
export const MAIN_CONFIG_ID = 'default';

/**
 * Copies a Claude Code profile (or the main config, MAIN_CONFIG_ID) into a new
 * profile and lands on it. Resolves false on failure so the dialog stays open.
 */
export function useDuplicateProfile(): (sourceId: string, name: string) => Promise<boolean> {
    const { t } = useTranslation();
    const notify = useNotify();
    const navigate = useNavigate();
    const { refresh } = useProfileContext();

    return useCallback(async (sourceId: string, name: string) => {
        try {
            const result = await api.duplicateProfile(BASE_SCENARIO, sourceId, name);
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
    }, [notify, t, navigate, refresh]);
}
