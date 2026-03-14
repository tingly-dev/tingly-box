import { useEffect, useState, useRef } from 'react';
import { getBaseUrl } from '@/services/api.ts';
import { useHeaderHeight } from '../../../hooks/useHeaderHeight.ts';

/**
 * Hook for managing shared scenario page state and data loading
 * Handles base URL loading and header height measurement
 */
export const useScenarioPageData = (providers: any[], dependencies: any[] = []) => {
    const headerRef = useRef<HTMLDivElement>(null);
    const [baseUrl, setBaseUrl] = useState<string>('');

    const headerHeight = useHeaderHeight(
        headerRef,
        providers.length > 0,
        dependencies
    );

    useEffect(() => {
        let isMounted = true;

        const loadBaseUrl = async () => {
            const url = await getBaseUrl();
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
        headerRef,
        baseUrl,
        headerHeight,
    };
};
