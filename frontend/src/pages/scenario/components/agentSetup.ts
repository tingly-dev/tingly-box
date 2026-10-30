import { SPOTLIGHT_ADD_MODEL_EVENT } from '@/components/nodes/ActionAddNode';

export interface AgentApplyResult {
    success: boolean;
    files?: string[];
    createdFiles?: string[];
    updatedFiles?: string[];
    backupPaths?: string[];
    error?: string;
}

export interface AgentInstallAction {
    label: string;
    href: string;
    variant?: 'contained' | 'outlined' | 'text';
    external?: boolean;
}

/** True iff at least one rule has a service with both a non-empty provider and model. */
export const hasModelOnAnyRule = (rules: any[] | null | undefined): boolean =>
    Array.isArray(rules) &&
    rules.some(r => Array.isArray(r?.services) && r.services.some((s: any) => s?.provider && s?.model));

/**
 * Smoothly scroll the "Model Rules" card into view, then
 * spotlight the "+ Add model" target so "Select a Model" actually points the
 * user at where to click — not just near it. The pulse is fired after the
 * scroll settles so it lands in view.
 */
export const scrollToModelsCard = () => {
    document.getElementById('models-and-forwarding-rules')?.scrollIntoView({
        behavior: 'smooth',
        block: 'start',
    });
    window.setTimeout(() => {
        window.dispatchEvent(new CustomEvent(SPOTLIGHT_ADD_MODEL_EVENT));
    }, 450);
};
