import { afterEach, describe, expect, it } from 'vitest';
import { armWorkspaceImHint, disarmWorkspaceImHint, dismissWorkspaceImHint, imHintArmed, imHintDismissed } from './imHint';

afterEach(() => {
    window.localStorage.clear();
});

describe('imHint', () => {
    it('arms until shown, and never again once dismissed', () => {
        armWorkspaceImHint();
        expect(imHintArmed()).toBe(true);
        expect(imHintDismissed()).toBe(false);

        dismissWorkspaceImHint();
        disarmWorkspaceImHint();
        expect(imHintArmed()).toBe(false);
        expect(imHintDismissed()).toBe(true);

        // Enabling the power-up again must not arm a dismissed hint.
        armWorkspaceImHint();
        expect(imHintArmed()).toBe(false);
    });
});
