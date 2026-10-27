import React from 'react';
import { api } from '@/services/api';
import type { CodexWindow, OpenAIModelsResponse } from '@/types/model';
import { codexGuessWindow, type CodexResolvedWindow } from './codexCompact';

/**
 * `known`: Codex's model list has been read for this model. `pending`: not
 * read yet. `unavailable`: the read failed. Until it is known, off and on are
 * what Auto writes for a model it doesn't know.
 */
export type CodexCatalogWindowStatus = 'known' | 'pending' | 'unavailable';

export interface CodexCatalogWindow {
    /** What the rule's "Auto" writes with its 1M flag off. */
    off: CodexResolvedWindow;
    /** What the rule's "Auto" writes with its 1M flag on. */
    on: CodexResolvedWindow;
    status: CodexCatalogWindowStatus;
}

type Answer = Pick<CodexCatalogWindow, 'off' | 'on'>;

const GUESS: Answer = { off: codexGuessWindow(false), on: codexGuessWindow(true) };
const SOURCES: Record<CodexWindow['source'], true> = { catalog: true, extended: true, default: true };
const isSource = (raw: unknown): raw is CodexWindow['source'] => typeof raw === 'string' && Object.hasOwn(SOURCES, raw);

// A token count from the list, 0 when absent or not a positive integer.
const tokens = (raw: unknown) => (typeof raw === 'number' && Number.isInteger(raw) && raw > 0 ? raw : 0);

// One side of codex_windows, or null when it isn't a usable window.
function readWindow(raw: unknown): CodexResolvedWindow | null {
    if (!raw || typeof raw !== 'object') return null;
    const w = raw as Record<string, unknown>;
    const window = tokens(w.context_window);
    if (!window || !isSource(w.source)) return null;
    return { window, max: Math.max(window, tokens(w.max_context_window)), source: w.source };
}

// Last answer per request model, shared by the Context and Compact chips and
// the 1M menu and kept across opens, so a reopen shows the last known value at
// once while it is read again.
const answers = new Map<string, Answer>();

/** For tests. */
export function clearCodexCatalogWindowCache() {
    answers.clear();
}

/**
 * What the rule's "Auto" writes into Codex's model catalog for its model, with
 * the 1M flag off and on, as Codex's model list (/tingly/codex/v1/models,
 * matched by request model) carries it in codex_windows. The server works it
 * out from the route (config.CodexAutoContextWindow) and already merges rules
 * that share a request model, so this only reads it. Read when a popover or
 * menu opens, so it follows service changes made since.
 */
export function useCodexCatalogWindow(requestModel: string | undefined, open: boolean): CodexCatalogWindow {
    const [failed, setFailed] = React.useState<string | null>(null);
    const [, rerender] = React.useReducer((n: number) => n + 1, 0);
    React.useEffect(() => {
        if (!open || !requestModel) return;
        let cancelled = false;
        (async () => {
            try {
                const list = await api.listScenarioModels('codex') as OpenAIModelsResponse | undefined;
                // The model API answers a failure ({success: false}, an error
                // body) instead of throwing.
                if (!Array.isArray(list?.data)) throw new Error('no model list');
                if (cancelled) return;
                const windows = list.data.find((m) => m.id === requestModel && m.codex_windows)?.codex_windows;
                answers.set(requestModel, {
                    off: readWindow(windows?.off) ?? GUESS.off,
                    on: readWindow(windows?.on) ?? GUESS.on,
                });
                setFailed(null);
                rerender();
            } catch {
                // Keep the last answer if there is one.
                if (!cancelled) setFailed(requestModel);
            }
        })();
        return () => { cancelled = true; };
    }, [open, requestModel]);

    // Without a request model there is no list entry to read: unknown.
    if (!requestModel) return { ...GUESS, status: 'known' };
    const known = answers.get(requestModel);
    if (known) return { ...known, status: 'known' };
    return { ...GUESS, status: failed === requestModel ? 'unavailable' : 'pending' };
}
