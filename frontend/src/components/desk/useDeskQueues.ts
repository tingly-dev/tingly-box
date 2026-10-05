import {useDeskStorage} from './useDeskStorage';

export interface DeskQueue {
    items: string[];
    held?: 'restored' | 'failed' | 'paused';
    inFlight?: boolean;
}
type Queues = Record<string, DeskQueue>;

const decode = (saved: unknown): Queues => {
    if (!saved || typeof saved !== 'object' || Array.isArray(saved)) return {};
    return Object.fromEntries(Object.entries(saved).flatMap(([id, value]) => {
        if (!value || typeof value !== 'object' || !('items' in value) || !Array.isArray(value.items)) return [];
        const items = value.items.filter((text: unknown): text is string => typeof text === 'string' && text.trim() !== '');
        return items.length ? [[id, {items, held: 'restored' as const}]] : [];
    }));
};

// Keep text until acceptance. On a browser reload the previous POST's outcome
// is unknown, so decoded queues are held for an explicit review and send.
export function useDeskQueues() {
    return useDeskStorage('desk.sessionQueues', decode);
}
