/**
 * Model name utilities for 1M context window feature
 */

/**
 * Formats a model name with [1m] suffix if the context_1m flag is enabled
 * @param model - The base model name
 * @param flags - Rule flags to check for context_1m
 * @returns Model name with [1m] suffix if context_1m is enabled, otherwise original model name
 */
export function formatModelNameWithContext1M(model: string, flags?: any): string {
    if (!model || !flags || !flags.context1m) {
        return model;
    }

    // Only add suffix if not already present
    if (!model.endsWith('[1m]')) {
        return `${model}[1m]`;
    }

    return model;
}

/**
 * Strips the [1m] suffix from a model name if present
 * @param model - Model name that may contain [1m] suffix
 * @returns Model name without [1m] suffix
 */
export function stripContext1MSuffix(model: string): string {
    if (!model) {
        return model;
    }

    if (model.endsWith('[1m]')) {
        return model.slice(0, -4); // Remove '[1m]'
    }

    return model;
}

/**
 * Checks if a model name has the 1M context suffix
 * @param model - Model name to check
 * @returns true if model name ends with [1m], false otherwise
 */
export function hasContext1MSuffix(model: string): boolean {
    return model && model.endsWith('[1m]');
}
