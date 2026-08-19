import React, {createContext, useCallback, useContext, useEffect, useState} from 'react';
import type {ReactNode} from 'react';
import {api} from '@/services/api';
import type {Team} from '@/types/team';

interface TeamContextType {
    teams: Team[];
    loading: boolean;
    refresh: () => Promise<void>;
}

const TeamContext = createContext<TeamContextType | undefined>(undefined);

export const useTeamContext = () => {
    const context = useContext(TeamContext);
    if (!context) throw new Error('useTeamContext must be used within TeamProvider');
    return context;
};

export const TeamProvider: React.FC<{children: ReactNode}> = ({children}) => {
    const [teams, setTeams] = useState<Team[]>([]);
    const [loading, setLoading] = useState(true);

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            const result = await api.listTeams();
            if (result.success) setTeams(result.data?.teams || []);
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => { void refresh(); }, [refresh]);

    return <TeamContext.Provider value={{teams, loading, refresh}}>{children}</TeamContext.Provider>;
};
