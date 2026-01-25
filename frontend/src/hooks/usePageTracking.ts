/**
 * usePageTracking Hook
 *
 * Automatically tracks page views when route changes
 * Only sends events if user has given consent
 */

import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { analytics } from '@/utils/analytics';

export const usePageTracking = () => {
    const location = useLocation();

    useEffect(() => {
        // Track page view when route changes
        analytics.trackPageView(location.pathname, document.title);
    }, [location.pathname]);
};
