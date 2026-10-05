import {useDeskStorage} from './useDeskStorage';

const decode = (value: unknown): Record<string, string> => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
    return Object.fromEntries(Object.entries(value).filter(([, text]) => typeof text === 'string'));
};

// Tab-scoped storage survives reloads without copying prompts to other tabs.
export function useDeskDrafts(key: string) {
    const [drafts, setDrafts] = useDeskStorage(key, decode);
    return [drafts, setDrafts] as const;
}
