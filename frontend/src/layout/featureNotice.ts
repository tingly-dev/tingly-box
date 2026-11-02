// Shared by the sidebar shortcut and the Power-ups settings switches.
export const FEATURE_DISABLED_EVENT = 'layout-feature-disabled';

export function announceFeatureDisabled(name: string) {
    window.dispatchEvent(new CustomEvent(FEATURE_DISABLED_EVENT, { detail: name }));
}
