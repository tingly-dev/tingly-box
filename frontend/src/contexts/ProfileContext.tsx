import React, { createContext, useCallback, useContext, useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { api } from '@/services/api';
import { PROFILE_SCENARIOS } from '@/constants/profileScenarios';

export interface ProfileInfo {
    id: string;
    name: string;
    unified?: boolean;  // true=unified mode, false/undefined=separate mode
}

interface ScenarioProfiles {
    [scenario: string]: ProfileInfo[];
}

interface ProfileContextType {
    /** Map of base scenario -> profiles array */
    profiles: ScenarioProfiles;
    /** Whether profiles are being loaded */
    loading: boolean;
    /** Force reload all profiles */
    refresh: () => void;
    /** Convenience: get profiles for a specific scenario */
    getProfiles: (scenario: string) => ProfileInfo[];
    /** Scenarios that support profiles, from backend descriptors */
    profileScenarios: string[];
}

const ProfileContext = createContext<ProfileContextType | undefined>(undefined);

export const useProfileContext = () => {
    const context = useContext(ProfileContext);
    if (!context) {
        throw new Error('useProfileContext must be used within ProfileProvider');
    }
    return context;
};

interface ProfileProviderProps {
    children: ReactNode;
}

export const ProfileProvider: React.FC<ProfileProviderProps> = ({ children }) => {
    const [profiles, setProfiles] = useState<ScenarioProfiles>({});
    const [loading, setLoading] = useState(true);
    const [profileScenarios, setProfileScenarios] = useState<string[]>([...PROFILE_SCENARIOS]);

    const loadProfiles = useCallback(async () => {
        setLoading(true);
        try {
            // Fetch descriptor list from backend; fall back to static list on error
            let scenarios: string[] = [...PROFILE_SCENARIOS];
            try {
                const descriptorResult = await api.getScenarioDescriptors();
                if (descriptorResult.success && Array.isArray(descriptorResult.data)) {
                    const fromBackend = (descriptorResult.data as Array<{ id: string; supports_profiles?: boolean }>)
                        .filter(d => d.supports_profiles)
                        .map(d => d.id);
                    if (fromBackend.length > 0) {
                        scenarios = fromBackend;
                    }
                }
            } catch {
                // Keep static fallback
            }
            setProfileScenarios(scenarios);

            const results = await Promise.allSettled(
                scenarios.map(async (scenario) => {
                    const result = await api.getProfiles(scenario);
                    if (result.success && Array.isArray(result.data) && result.data.length > 0) {
                        return { scenario, data: result.data as ProfileInfo[] };
                    }
                    return null;
                })
            );
            const map: ScenarioProfiles = {};
            for (const r of results) {
                if (r.status === 'fulfilled' && r.value) {
                    map[r.value.scenario] = r.value.data;
                }
            }
            setProfiles(map);
        } catch {
            // Silently ignore
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        loadProfiles();
    }, [loadProfiles]);

    const getProfiles = useCallback((scenario: string): ProfileInfo[] => {
        return profiles[scenario] || [];
    }, [profiles]);

    return (
        <ProfileContext.Provider value={{ profiles, loading, refresh: loadProfiles, getProfiles, profileScenarios }}>
            {children}
        </ProfileContext.Provider>
    );
};
