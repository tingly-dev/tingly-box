import React, { createContext, useContext, useEffect, useState, ReactNode } from 'react';
import { api } from '@/services/api';
import { useAuth } from './AuthContext';

interface FeatureFlagsContextType {
    skillUser: boolean;
    skillIde: boolean;
    enableGuardrails: boolean;
    enableMCP: boolean;
    loading: boolean;
    refresh: () => void;
}

const FeatureFlagsContext = createContext<FeatureFlagsContextType | undefined>(undefined);

export const useFeatureFlags = () => {
    const context = useContext(FeatureFlagsContext);
    if (!context) {
        throw new Error('useFeatureFlags must be used within FeatureFlagsProvider');
    }
    return context;
};

interface FeatureFlagsProviderProps {
    children: ReactNode;
}

export const FeatureFlagsProvider: React.FC<FeatureFlagsProviderProps> = ({ children }) => {
    const { isLoading: isAuthLoading } = useAuth();
    const [skillUser, setSkillUser] = useState(false);
    const [skillIde, setSkillIde] = useState(false);
    const [enableGuardrails, setEnableGuardrails] = useState(false);
    const [enableMCP, setEnableMCP] = useState(false);
    const [loading, setLoading] = useState(true);

    const loadFlags = async () => {
        try {
            const [skillUserResult, skillIdeResult, guardrailsResult, mcpResult] = await Promise.all([
                api.getScenarioFlag('_global', 'skill_user'),
                api.getScenarioFlag('_global', 'skill_ide'),
                api.getScenarioFlag('_global', 'guardrails'),
                api.getScenarioFlag('_global', 'mcp'),
            ]);
            setSkillUser(skillUserResult?.data?.value || false);
            setSkillIde(skillIdeResult?.data?.value || false);
            setEnableGuardrails(guardrailsResult?.data?.value || false);
            setEnableMCP(mcpResult?.data?.value || false);
        } catch (error) {
            // Silently fail - flags will default to false
            // Don't log to console to avoid noise during initial auth
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        // Only load flags after auth initialization is complete
        // This prevents 401 errors during initial load which would clear the token
        if (!isAuthLoading) {
            loadFlags();
        }
    }, [isAuthLoading]);

    const refresh = () => {
        loadFlags();
    };

    return (
        <FeatureFlagsContext.Provider value={{ skillUser, skillIde, enableGuardrails, enableMCP, loading, refresh }}>
            {children}
        </FeatureFlagsContext.Provider>
    );
};
