import { useCallback, useEffect, useRef } from 'react';
import { Alert, Box, Button } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams } from 'react-router-dom';
import { useRuleManagement } from '@/pages/scenario/hooks/useRuleManagement';
import { useNotify } from '@/hooks/useNotify';
import ImageGenPlaygroundCard from './components/ImageGenPlaygroundCard';
import { createImageProfile, useImageProfiles, useImageProfilesReady } from './profiles/imageProfileStore';

const scenario = 'imagegen';
const WORKBENCH_MIN_HEIGHT = 600;

// One image profile, one page — the same workbench as the Playground, with
// the profile's references, description and settings already in place
// (the Claude Code profile pages follow the same shape).
const ImageProfilePage: React.FC = () => {
    const { profileId = '' } = useParams<{ profileId: string }>();
    const { t } = useTranslation();
    const navigate = useNavigate();
    const profiles = useImageProfiles();
    const ready = useImageProfilesReady();
    const profile = profiles.find((item) => item.id === profileId);
    const { rules, loadingRule, loadRules } = useRuleManagement();
    const { notify } = useNotify();

    useEffect(() => {
        void loadRules(scenario);
    }, [loadRules]);

    // The sidebar's "New profile" lands here: create an empty one and move to
    // its page, title open for naming. No dialog in between.
    // Guarded: StrictMode runs effects twice in development, and this one
    // creates something.
    const creatingRef = useRef(false);
    useEffect(() => {
        // Re-armed once we have left "new", so a later click creates again.
        if (profileId !== 'new') { creatingRef.current = false; return; }
        if (creatingRef.current) return;
        creatingRef.current = true;
        const created = createImageProfile({
            name: t('imageProfile.untitled', { defaultValue: 'Untitled profile' }),
            refs: [],
            prompts: [{ id: 'p1', name: '', text: '' }],
            activePromptId: 'p1',
            model: '',
            size: '1024x1024',
            quality: 'auto',
            count: 1,
        });
        navigate(`/image/profile/${created.id}`, { replace: true, state: { rename: true } });
    }, [navigate, profileId, t]);

    const showNotification = useCallback(
        (message: string, severity: 'success' | 'info' | 'warning' | 'error') => { notify(severity, message); },
        [notify],
    );

    // Still reading stored profiles (a reload lands here): not "gone" yet.
    if (profileId === 'new' || (!profile && !ready)) return null;

    if (!profile) {
        return (
            <Alert
                severity="info"
                variant="outlined"
                action={(
                    <Button color="inherit" size="small" onClick={() => navigate('/image/playground')}>
                        {t('imageProfile.backToPlayground', { defaultValue: 'Open Playground' })}
                    </Button>
                )}
            >
                {t('imageProfile.notFound', { defaultValue: 'This profile no longer exists.' })}
            </Alert>
        );
    }

    return (
        <Box sx={{ height: { lg: '100%' }, minHeight: { lg: WORKBENCH_MIN_HEIGHT } }}>
            {/* Keyed by profile: switching profiles starts a fresh panel from
                that profile's settings instead of carrying the last one's. */}
            <ImageGenPlaygroundCard
                key={profile.id}
                profile={profile}
                rules={rules}
                loadingRules={loadingRule}
                showNotification={showNotification}
            />
        </Box>
    );
};

export default ImageProfilePage;
