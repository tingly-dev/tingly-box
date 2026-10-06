import { useMemo, useRef, useSyncExternalStore } from 'react';

// The prompt text, kept outside the card's React state. The playground card is
// large; if the text were its state, every keystroke would re-render all of it.
// Here only the components that subscribe (the field, the copy button, the
// editor dialog) re-render per keystroke; the card reads the text on demand
// and re-renders only when something it shows actually changes, such as the
// prompt going from empty to filled.
export interface PromptStore {
    get: () => string;
    set: (text: string) => void;
    subscribe: (listener: () => void) => () => void;
}

export const usePromptStore = (initial: string): PromptStore => {
    const initialRef = useRef(initial);
    return useMemo(() => {
        let text = initialRef.current;
        const listeners = new Set<() => void>();
        return {
            get: () => text,
            set: (next) => {
                if (next === text) return;
                text = next;
                listeners.forEach((listener) => listener());
            },
            subscribe: (listener) => {
                listeners.add(listener);
                return () => { listeners.delete(listener); };
            },
        };
    }, []);
};

// Re-renders on every change — for the field and anything else that shows the text.
export const usePromptText = (store: PromptStore): string => (
    useSyncExternalStore(store.subscribe, store.get, store.get)
);

// Re-renders only when the prompt flips between empty and not.
export const usePromptFilled = (store: PromptStore): boolean => (
    useSyncExternalStore(store.subscribe, () => store.get().trim() !== '', () => store.get().trim() !== '')
);
