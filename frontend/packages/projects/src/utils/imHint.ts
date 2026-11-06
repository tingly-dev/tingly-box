// The Workspace home's first-visit IM hint is armed by the act of enabling
// the Workspace power-up, not by page visits: the host calls armWorkspaceImHint
// when the user turns the feature on (the gate's Enable dialog or the
// experimental settings page), and the next Workspace home render pops the
// hint once. Once dismissed it stays away for good, even if the feature is
// enabled again.
import { readLocal, removeLocal, writeLocal } from './storage';

const ARMED = 'workspace.imHintArmed';
const DISMISSED = 'workspace.imHintDone';

/** The host reports the feature was just enabled: the next Workspace home render shows the hint (unless it was dismissed). */
export function armWorkspaceImHint() {
    if (readLocal(DISMISSED) !== '1') writeLocal(ARMED, '1');
}

export function imHintArmed(): boolean {
    return readLocal(ARMED) === '1';
}

export function imHintDismissed(): boolean {
    return readLocal(DISMISSED) === '1';
}

/** The hint was shown: it must not come back until the feature is enabled again. */
export function disarmWorkspaceImHint() {
    removeLocal(ARMED);
}

export function dismissWorkspaceImHint() {
    writeLocal(DISMISSED, '1');
}
