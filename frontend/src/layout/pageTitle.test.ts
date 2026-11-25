import { describe, expect, it } from 'vitest';
import { pageLabel } from './pageTitle';
import type { ActivityItem } from './types';

const items: ActivityItem[] = [
    { key: 'credential', icon: null, label: 'Credentials', children: [{ path: '/credentials', label: 'Model Key' }] },
    { key: 'dashboard', icon: null, label: 'Dashboard', children: [{ path: '/dashboard/today', label: 'Usage', match: p => p.startsWith('/dashboard/') }] },
];

describe('pageLabel', () => {
    it('uses the nav row that claims the path', () => {
        expect(pageLabel(items, '/credentials', 'credential', 'Bots')).toBe('Model Key');
        expect(pageLabel(items, '/dashboard/7d', 'dashboard', 'Bots')).toBe('Usage');
    });
    it('falls back to the activity, and names bots pages', () => {
        expect(pageLabel(items, '/nowhere', 'dashboard', 'Bots')).toBe('Dashboard');
        expect(pageLabel(items, '/bots/overview', 'dashboard', 'Bots')).toBe('Bots');
    });
});
