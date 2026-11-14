import { useSyncExternalStore } from 'react';
import { useIsPhone } from '@tingly/ui/hooks/useIsPhone';

export type ShellPref = 'auto' | 'mobile' | 'desktop';
export type Shell = 'mobile' | 'desktop';

const STORAGE_KEY = 'ui.shell';

// The phone shell is not finished: until it is, only an explicit choice
// (`?ui=mobile`, or the switch in 更多) selects it and `auto` stays desktop.
export const MOBILE_AUTO = false;

/** What the interface picker offers: `auto` only once it means something other than desktop. */
export function shellChoices(): ShellPref[] {
    return MOBILE_AUTO ? ['auto', 'mobile', 'desktop'] : ['desktop', 'mobile'];
}

/** The choice to show as selected: an `auto` the picker does not offer reads as desktop, which is what it does. */
export function shownChoice(pref: ShellPref): ShellPref {
    return shellChoices().includes(pref) ? pref : 'desktop';
}

export function parseShellPref(value: string | null | undefined): ShellPref | null {
    return value === 'auto' || value === 'mobile' || value === 'desktop' ? value : null;
}

export function resolveShell(pref: ShellPref, isPhone: boolean): Shell {
    if (pref === 'mobile' || pref === 'desktop') return pref;
    return MOBILE_AUTO && isPhone ? 'mobile' : 'desktop';
}

function persist(pref: ShellPref) {
    try {
        localStorage.setItem(STORAGE_KEY, pref);
    } catch {
        // Storage unavailable: the choice lasts until reload.
    }
}

// A `?ui=` in the URL wins and is remembered; otherwise the stored choice.
function readPref(): ShellPref {
    const fromUrl = parseShellPref(new URLSearchParams(window.location.search).get('ui'));
    if (fromUrl) {
        persist(fromUrl);
        return fromUrl;
    }
    try {
        return parseShellPref(localStorage.getItem(STORAGE_KEY)) ?? 'auto';
    } catch {
        return 'auto';
    }
}

let current: ShellPref | undefined;
const listeners = new Set<() => void>();
const subscribe = (listener: () => void) => {
    listeners.add(listener);
    return () => { listeners.delete(listener); };
};
const getSnapshot = () => (current ??= readPref());

export function setShellPref(pref: ShellPref) {
    current = pref;
    persist(pref);
    listeners.forEach((listener) => listener());
}

/** Which shell to mount, and the user's choice behind it. */
export function useUiShell(): { shell: Shell; pref: ShellPref } {
    const pref = useSyncExternalStore(subscribe, getSnapshot);
    const isPhone = useIsPhone();
    return { shell: resolveShell(pref, isPhone), pref };
}
