import React from 'react';
import { api } from '@/services/api';
import type { OpenAIModelsResponse } from '@/types/model';

/**
 * The input limit (max_input_tokens) Codex's model list
 * (/tingly/codex/v1/models) gives a rule's model, from the provider catalog
 * (0 = unknown): what the rule's "Auto" window resolves to. Codex's
 * context_window is this input budget, not the published context_window,
 * which for OpenAI models counts output too. Read when a popover opens, so it follows service
 * changes made since.
 */
export function useCodexCatalogWindow(requestModel: string | undefined, open: boolean): number {
    const [window, setWindow] = React.useState(0);
    React.useEffect(() => {
        if (!open || !requestModel) return;
        let cancelled = false;
        (async () => {
            try {
                const list = await api.listScenarioModels('codex') as OpenAIModelsResponse;
                const entry = list?.data?.find((m) => m.id === requestModel);
                const value = Number(entry?.max_input_tokens ?? entry?.context ?? 0);
                if (!cancelled) setWindow(Number.isFinite(value) ? value : 0);
            } catch {
                // Unknown: Auto falls back to the 1M flag or 200K.
            }
        })();
        return () => { cancelled = true; };
    }, [open, requestModel]);
    return window;
}
