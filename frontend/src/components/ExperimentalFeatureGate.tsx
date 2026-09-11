import { useFeatureFlags } from '@/contexts/FeatureFlagsContext';
import { Box, CircularProgress } from '@mui/material';
import type { ReactNode } from 'react';
import { Navigate, useLocation } from 'react-router-dom';

export type ExperimentalFeature = 'skill_user' | 'skill_ide' | 'guardrails' | 'mcp' | 'managed_agent';

export const buildExperimentalFeatureRedirect = (
    feature: ExperimentalFeature,
    returnTo: string,
) => {
    const search = new URLSearchParams({ feature, returnTo });
    return `/system/experimental?${search.toString()}`;
};

export const parseExperimentalFeature = (value: string | null): ExperimentalFeature | undefined => {
    if (value === 'skill_user' || value === 'skill_ide' || value === 'guardrails' || value === 'mcp' || value === 'managed_agent') {
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

    if (flags.loading) {
        return (
            <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
                <CircularProgress size={28} />
            </Box>
        );
    }

    const enabled = {
        skill_user: flags.skillUser,
        skill_ide: flags.skillIde,
        guardrails: flags.enableGuardrails,
        mcp: flags.enableMCP,
        managed_agent: flags.enableManagedAgent,
    }[feature];

    if (!enabled) {
        const returnTo = `${location.pathname}${location.search}${location.hash}`;
        return <Navigate to={buildExperimentalFeatureRedirect(feature, returnTo)} replace />;
    }

    return children;
};

export default ExperimentalFeatureGate;
