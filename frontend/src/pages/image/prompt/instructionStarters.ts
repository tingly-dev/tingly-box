import { useState } from 'react';

// Shared by the prompt tools (Image to Prompt, Idea to Prompt): each has an
// editable instructions field that starts from a default, and a few starters
// that fill it.

export const readLocal = (key: string): string | null => {
    try { return localStorage.getItem(key); } catch { return null; }
};
export const writeLocal = (key: string, value: string | null) => {
    try {
        if (value === null) localStorage.removeItem(key);
        else localStorage.setItem(key, value);
    } catch { /* kept for this visit only */ }
};

// Which starter the field still matches, so the menu can mark it and the page
// can tell an edited instruction from an untouched one.
export const matchStarter = <K extends string>(starters: Record<K, string>, instructions: string): K | null => {
    const text = instructions.trim();
    return (Object.keys(starters) as K[]).find((key) => starters[key] === text) ?? null;
};

// The field's text, saved only once it differs from the default, so a later,
// better default still reaches everyone who never touched it.
export const useInstructions = (storageKey: string, defaultText: string) => {
    const [instructions, setInstructions] = useState(() => readLocal(storageKey) ?? defaultText);
    const change = (next: string) => {
        setInstructions(next);
        writeLocal(storageKey, next.trim() === defaultText ? null : next);
    };
    return [instructions, change] as const;
};

// The model last chosen on a page, if it is still one of the rules.
export const useRememberedModel = (storageKey: string, models: string[]) => {
    const [selected, setSelected] = useState(() => readLocal(storageKey) ?? '');
    const model = models.includes(selected) ? selected : (models[0] ?? '');
    const choose = (next: string) => {
        setSelected(next);
        writeLocal(storageKey, next);
    };
    return [model, choose] as const;
};
