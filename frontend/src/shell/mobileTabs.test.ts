import { describe, expect, it } from 'vitest';
import type { ActivityItem } from '@/layout/types';
import { activeTab, buildTabs } from './mobileTabs';

const items: ActivityItem[] = [
    { key: 'scenario', icon: 'A', label: 'Agent', defaultPath: '/agent', children: [{ path: '/agent/claude_code', label: 'Claude Code' }] },
    { key: 'projects', icon: 'W', label: 'Workspace', defaultPath: '/workspace', attention: 'waiting', children: [{ path: '/workspace', label: 'Workspace', match: (p) => p === '/workspace' || p.startsWith('/projects/') }] },
    { key: 'credential', icon: 'C', label: 'Credentials', defaultPath: '/credentials', path: '/credentials' },
];
const more = { label: 'More', icon: 'M' };

describe('buildTabs', () => {
    it('lists Workspace, Agent, More from the activity list, with the waiting dot', () => {
        const tabs = buildTabs(items, more, true);
        expect(tabs.map((t) => [t.key, t.to, t.attention])).toEqual([
            ['projects', '/workspace', true],
            ['scenario', '/agent', false],
            ['more', '/more', false],
        ]);
    });

    it('leaves Workspace out while the feature is off', () => {
        expect(buildTabs(items, more, false).map((t) => t.key)).toEqual(['scenario', 'more']);
    });
});

describe('activeTab', () => {
    const tabs = buildTabs(items, more, true);
    it('lights Workspace and Agent for their pages and More for everything else', () => {
        expect(activeTab(tabs, items, '/projects/p1', null)).toBe('projects');
        expect(activeTab(tabs, items, '/agent/claude_code', null)).toBe('scenario');
        expect(activeTab(tabs, items, '/credentials', null)).toBe('more');
        expect(activeTab(tabs, items, '/more', 'projects')).toBe('more');
    });

    it('puts a Workspace path under More while the Workspace tab is off', () => {
        expect(activeTab(buildTabs(items, more, false), items, '/workspace', null)).toBe('more');
    });
});
