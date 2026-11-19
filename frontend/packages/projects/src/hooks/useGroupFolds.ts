import { useState } from 'react';
import { readLocal, writeLocal } from '../utils/storage';

// Which of the Overview's groups the user folded or opened, per project or
// folder and per browser. Only choices are stored; a group nobody touched
// follows its default, so live updates never move it. The version in the key
// lets a later change of the defaults start from a clean slate.
const foldKey = (scope: string) => `overview.folds.v1.${scope}`;

/** Group key → true when the user folded it, false when they opened it. */
export type GroupFolds = Record<string, boolean>;

function readFolds(scope: string): GroupFolds {
    try {
        const v: unknown = JSON.parse(readLocal(foldKey(scope)) ?? '{}');
        if (v == null || typeof v !== 'object' || Array.isArray(v)) return {};
        return Object.fromEntries(Object.entries(v).filter((e): e is [string, boolean] => typeof e[1] === 'boolean'));
    } catch {
        return {};
    }
}

/** The user's fold choices for a scope, and a setter that remembers them. */
export function useGroupFolds(scope: string): [GroupFolds, (keys: string[], folded: boolean) => void] {
    // Kept per scope: the pane can switch projects without remounting.
    const [all, setAll] = useState<Record<string, GroupFolds>>(() => ({ [scope]: readFolds(scope) }));
    const folds = all[scope] ?? readFolds(scope);
    const set = (keys: string[], folded: boolean) => setAll((cur) => {
        const next = { ...(cur[scope] ?? readFolds(scope)) };
        for (const k of keys) next[k] = folded;
        writeLocal(foldKey(scope), JSON.stringify(next));
        return { ...cur, [scope]: next };
    });
    return [folds, set];
}
