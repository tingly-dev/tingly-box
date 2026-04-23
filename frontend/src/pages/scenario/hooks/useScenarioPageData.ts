import { useEffect, useState } from 'react';
import {getApiBaseUrl} from '@/utils/protocol';

/**
 * Hook for managing shared scenario page state and data loading
 * Handles base URL loading
 */
export const useScenarioPageData = (providers: any[], dependencies: any[] = []) => {
    const [baseUrl, setBaseUrl] = useState<string>('');

    useEffect(() => {
        let isMounted = true;

        const loadBaseUrl = async () => {
            const url = await getApiBaseUrl();
            if (isMounted) {
                setBaseUrl(url);
            }
        };

        loadBaseUrl();

        return () => {
            isMounted = false;
        };
    }, []);

    return {
        baseUrl,
    };
};
