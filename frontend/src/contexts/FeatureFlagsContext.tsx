import React, { createContext, useContext, useEffect, useState, ReactNode } from 'react';
import { api } from '@/services/api';

interface FeatureFlagsContextType {
    skillUser: boolean;
    skillIde: boolean;
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
    const [skillUser, setSkillUser] = useState(false);
    const [skillIde, setSkillIde] = useState(false);
    const [loading, setLoading] = useState(true);

    const loadFlags = async () => {
        try {
            const [skillUserResult, skillIdeResult] = await Promise.all([
                api.getScenarioFlag('_global', 'skill_user'),
                api.getScenarioFlag('_global', 'skill_ide'),
            ]);
            setSkillUser(skillUserResult?.data?.value || false);
            setSkillIde(skillIdeResult?.data?.value || false);
        } catch (error) {
            console.error('Failed to load feature flags:', error);
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        loadFlags();
    }, []);

    const refresh = () => {
        loadFlags();
    };

    return (
        <FeatureFlagsContext.Provider value={{ skillUser, skillIde, loading, refresh }}>
            {children}
        </FeatureFlagsContext.Provider>
    );
};
