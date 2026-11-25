import type { ActivityItem } from './types';

/**
 * The name of the page a path shows, for the browser tab: the nav row that
 * claims the path (a profile's own name, a dashboard range's "Usage"), else the
 * activity's own name. Pages with a more specific focus (an open thread)
 * name the tab themselves.
 */
export function pageLabel(items: ActivityItem[], pathname: string, activeKey: string, botsLabel: string): string {
    if (pathname === '/bots' || pathname.startsWith('/bots/')) return botsLabel;
    for (const item of items) {
        if (item.path === pathname) return item.label;
        for (const child of item.children ?? []) {
            if (child.type === 'divider') continue;
            if (child.match ? child.match(pathname) : child.path === pathname) return child.label;
        }
    }
    return items.find(item => item.key === activeKey)?.label ?? '';
}
