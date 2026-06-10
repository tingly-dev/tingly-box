/**
 * Model name utilities for the 1M context window feature.
 *
 * The [1m] advertisement is part of the model string itself; the suffix
 * lives on the wire, not as a separate field. These helpers are the single
 * frontend source of truth for adding/removing/checking it.
 */

const ONE_M_SUFFIX = '[1m]';

/** Returns true if the model name carries the [1m] suffix. */
export function has1M(model: string | undefined): boolean {
    return !!model && model.endsWith(ONE_M_SUFFIX);
}

/** Returns the model name with the [1m] suffix toggled on or off. */
export function with1M(model: string | undefined, on: boolean): string {
    const base = (model || '').replace(/\[1m\]$/, '');
    return on && base !== '' ? base + ONE_M_SUFFIX : base;
}

/**
 * Formats a model name with the [1m] suffix when the rule's context_1m flag
 * (camelCase RuleFlags shape) is enabled; display helper for rule cards.
 */
export function formatModelNameWithContext1M(model: string, flags?: any): string {
    if (!model || !flags?.context1m) {
        return model;
    }
    return with1M(model, true);
}
