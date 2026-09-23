/**
 * Blur the currently focused element.
 *
 * MUI restores focus to the dialog trigger after a dialog closes, which leaves
 * toolbar buttons showing the white focus overlay. Call this on dialog
 * open/close paths so controls return to a neutral state.
 */
export const blurActiveElement = () => {
    const active = document.activeElement;
    if (active instanceof HTMLElement) {
        active.blur();
    }
};
