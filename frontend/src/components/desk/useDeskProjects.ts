import {useLocalStorage} from '@/hooks/useLocalStorage';

export const PROJECTS_KEY = 'desk.projectDirectories:v1';
const EMPTY: Record<string, boolean> = {};

export const isAbsoluteProjectPath = (path: string): boolean =>
    path.startsWith('/') || /^[a-z]:[\\/]/i.test(path) || /^\\\\[^\\]+\\[^\\]+/.test(path);

export const normalizeProjectPath = (path: string): string => {
    const trimmed = path.trim();
    if (/^[a-z]:[\\/]$/i.test(trimmed)) return trimmed;
    return trimmed.replace(trimmed.startsWith('/') ? /\/+$/ : /[\\/]+$/, '') || (trimmed.startsWith('/') ? '/' : '');
};

const options = {
    deserializer: (raw: string): Record<string, boolean> => {
        const parsed: unknown = JSON.parse(raw);
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {};
        return Object.fromEntries(Object.keys(parsed).map(normalizeProjectPath).filter(isAbsoluteProjectPath).map((path) => [path, true]));
    },
};

// Browser shortcuts, independent of task creation. Actual filesystem checks
// remain the existing backend's responsibility when a task starts.
export function useDeskProjects() {
    const {data, saveData, removeKey, refetch} = useLocalStorage(PROJECTS_KEY, EMPTY, options);
    const add = (value: string): boolean => {
        const path = normalizeProjectPath(value);
        if (!isAbsoluteProjectPath(path) || !saveData(path, true)) return false;
        refetch();
        return true;
    };
    const remove = (path: string) => {
        const saved = removeKey(path);
        refetch();
        return saved;
    };
    return {projects: Object.keys(data).reverse(), addProject: add, removeProject: remove};
}
