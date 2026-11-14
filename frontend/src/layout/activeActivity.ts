import type { ActivityItem } from './types';

/**
 * The activity a path belongs to: the one whose own path it is, or whose
 * pages claim it. A path no activity claims keeps the last one the user was
 * in (`saved`), and without one it is the Agent activity.
 */
export function resolveActiveActivity(items: ActivityItem[], pathname: string, saved: string | null): string {
    if (pathname === '/help') return 'help';
    const isActive = (path: string) => pathname === path;
    const isChildActive = (children?: ActivityItem['children']) =>
        children?.some(item => item.type !== 'divider' && (item.match ? item.match(pathname) : isActive(item.path))) ?? false;
    for (const item of items) {
        if (item.path && isActive(item.path)) return item.key;
        if (item.children && isChildActive(item.children)) return item.key;
    }
    if (saved && items.some(item => item.key === saved)) return saved;
    return 'scenario';
}
