import {useLocalStorage} from '@/hooks/useLocalStorage';

export const PROMPTS_KEY = 'desk.promptLibrary:v1';

export interface DeskPrompt {
    id: string;
    name: string;
    text: string;
    createdAt: number;
    builtin?: boolean;
    // i18n key for a built-in's display name; `name` is the English fallback.
    nameKey?: string;
}

// Shipped with the page, read-only. "Copy to mine" turns one into a saved prompt.
export const BUILTIN_PROMPTS: DeskPrompt[] = [
    {id: 'builtin-review', nameKey: 'desk.library.builtin.review', name: 'Review changes', createdAt: 0, builtin: true,
        text: 'Review the uncommitted changes in this repo. List correctness bugs first, then risky spots, then nits.'},
    {id: 'builtin-fix-tests', nameKey: 'desk.library.builtin.fixTests', name: 'Run tests and fix', createdAt: 0, builtin: true,
        text: 'Run the test suite. For each failure, find the root cause and fix it, then rerun until green. Do not skip or disable tests.'},
    {id: 'builtin-explain', nameKey: 'desk.library.builtin.explain', name: 'Explain this repo', createdAt: 0, builtin: true,
        text: 'Give me a map of this repo: entry points, main modules, how to build and run it, and where to start reading.'},
    {id: 'builtin-commit', nameKey: 'desk.library.builtin.commit', name: 'Write commit message', createdAt: 0, builtin: true,
        text: "Look at the staged changes and propose a commit message following this repo's conventions. Do not commit."},
];

const EMPTY: Record<string, Omit<DeskPrompt, 'id'>> = {};

const options = {
    deserializer: (raw: string): Record<string, Omit<DeskPrompt, 'id'>> => {
        const parsed: unknown = JSON.parse(raw);
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {};
        return Object.fromEntries(Object.entries(parsed as Record<string, Partial<DeskPrompt>>)
            .filter(([, p]) => p && typeof p.name === 'string' && typeof p.text === 'string')
            .map(([id, p]) => [id, {name: p.name!, text: p.text!, createdAt: Number(p.createdAt) || 0}]));
    },
};

// Prompts kept in this browser (same approach as the project shortcuts).
// Saved prompts come first, newest first, then the built-ins.
export function useDeskPrompts() {
    const {data, saveData, removeKey, refetch} = useLocalStorage(PROMPTS_KEY, EMPTY, options);
    const mine: DeskPrompt[] = Object.entries(data)
        .map(([id, p]) => ({id, ...p}))
        .sort((a, b) => b.createdAt - a.createdAt);
    // Resolves false when the browser refuses to store it, so the caller keeps its input.
    const save = (fields: {id?: string; name: string; text: string}): boolean => {
        const name = fields.name.trim();
        const text = fields.text.trim();
        if (!name || !text) return false;
        const id = fields.id ?? `p-${Math.random().toString(36).slice(2, 10)}`;
        const createdAt = fields.id ? data[fields.id]?.createdAt ?? Date.now() : Date.now();
        if (!saveData(id, {name, text, createdAt})) return false;
        refetch();
        return true;
    };
    const remove = (id: string): boolean => {
        const ok = removeKey(id);
        refetch();
        return ok;
    };
    return {prompts: [...mine, ...BUILTIN_PROMPTS], savePrompt: save, removePrompt: remove};
}
