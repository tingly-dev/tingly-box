import { useFeatureFlags } from '@/contexts/FeatureFlagsContext';
import { Box, Button, CircularProgress, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle } from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '@/services/api';
import type { ReactNode } from 'react';
import { Navigate, useLocation, useNavigate } from 'react-router-dom';
import { armWorkspaceImHint } from '@tingly/projects/workspace-im-hint';

export type ExperimentalFeature = 'skill_ide' | 'guardrails' | 'mcp' | 'bench' | 'projects';

export const buildExperimentalFeatureRedirect = (
    feature: ExperimentalFeature,
    returnTo: string,
) => {
    const search = new URLSearchParams({ feature, returnTo });
    return `/system/experimental?${search.toString()}`;
};

export const parseExperimentalFeature = (value: string | null): ExperimentalFeature | undefined => {
    if (value === 'skill_ide' || value === 'guardrails' || value === 'mcp' || value === 'bench' || value === 'projects') {
        return value;
    }
    return undefined;
};

export const sanitizeFeatureReturnPath = (value: string | null): string | undefined => {
    if (!value?.startsWith('/') || value.startsWith('//')) return undefined;
    return value;
};

interface ExperimentalFeatureGateProps {
    feature: ExperimentalFeature;
    children: ReactNode;
}

const ExperimentalFeatureGate = ({ feature, children }: ExperimentalFeatureGateProps) => {
    const location = useLocation();
    const flags = useFeatureFlags();
    const navigate = useNavigate();
    const { t } = useTranslation();
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(false);

    if (flags.loading) {
        return (
            <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
                <CircularProgress size={28} />
            </Box>
        );
    }

    const enabled = {
        skill_ide: flags.skillIde,
        guardrails: flags.enableGuardrails,
        mcp: flags.enableMCP,
        bench: flags.enableBench,
        projects: flags.enableProjects,
    }[feature];

    if (!enabled && feature === 'projects') {
        const enable = async () => {
            setBusy(true); setError(false);
            try {
                const result = await api.setScenarioFlag('_global', 'projects', true);
                if (!result.success) throw new Error('Feature update rejected');
                // First enablement: the Workspace home's IM hint pops once, there.
                armWorkspaceImHint();
                await flags.refresh();
            } catch { setError(true); } finally { setBusy(false); }
        };
        return <Dialog open onClose={() => navigate('/agent')}>
            <DialogTitle>{t('layout.workspaceGate.title', { defaultValue: 'Enable Workspace' })}</DialogTitle>
            <DialogContent><DialogContentText>{t('layout.workspaceGate.text', { defaultValue: 'Enable the Workspace power-up to use projects, workdirs and IM chats.' })}</DialogContentText>{error && <DialogContentText color="error">{t('layout.workspaceGate.error', { defaultValue: 'Could not enable Workspace. Please try again.' })}</DialogContentText>}</DialogContent>
            <DialogActions><Button disabled={busy} onClick={() => navigate('/agent')}>{t('common.cancel', { defaultValue: 'Cancel' })}</Button><Button disabled={busy} variant="contained" onClick={enable}>{t('layout.workspaceGate.enable', { defaultValue: 'Enable Workspace' })}</Button></DialogActions>
        </Dialog>;
    }
    if (!enabled) {
        const returnTo = `${location.pathname}${location.search}${location.hash}`;
        return <Navigate to={buildExperimentalFeatureRedirect(feature, returnTo)} replace />;
    }

    return children;
};

export default ExperimentalFeatureGate;
