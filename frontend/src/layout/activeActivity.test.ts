import { describe, expect, it } from 'vitest';
import { resolveActiveActivity } from './activeActivity';
import type { ActivityItem } from './types';

const items: ActivityItem[] = [
    { key: 'scenario', icon: null, label: 'Agent', defaultPath: '/agent', children: [{ path: '/agent/claude_code', label: 'Claude Code' }] },
    { key: 'projects', icon: null, label: 'Workspace', children: [{ path: '/workspace', label: 'Workspace', match: (p) => p === '/workspace' || p.startsWith('/projects/') }] },
    { key: 'credential', icon: null, label: 'Credentials', path: '/credentials' },
];

describe('resolveActiveActivity', () => {
    it('takes a child page, a match() and an activity path', () => {
        expect(resolveActiveActivity(items, '/agent/claude_code', null)).toBe('scenario');
        expect(resolveActiveActivity(items, '/projects/p1', null)).toBe('projects');
        expect(resolveActiveActivity(items, '/credentials', null)).toBe('credential');
    });

    it('keeps the saved activity for a path nobody claims, if it still exists', () => {
        expect(resolveActiveActivity(items, '/system', 'projects')).toBe('projects');
        expect(resolveActiveActivity(items, '/system', 'gone')).toBe('scenario');
    });

    it('treats /help as its own activity', () => {
        expect(resolveActiveActivity(items, '/help', 'projects')).toBe('help');
    });
});
