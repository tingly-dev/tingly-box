// What the library page hands the playground when the user picks "use" on a
// kept prompt, piece or image: carried in router state, read once on arrival.
// Images travel by id (the playground reads them from the library) because
// router state is written into session history, which is no place for
// megabytes of base64.

export interface PlaygroundHandoff {
    // Replaces the prompt field.
    prompt?: string;
    // Appended to whatever is in the prompt field — a term or a phrase is a
    // part of a prompt, not a whole one.
    piece?: string;
    referenceIds?: string[];
}

const KEY = 'imageLibraryHandoff';

export const playgroundHandoffState = (handoff: PlaygroundHandoff) => ({ [KEY]: handoff });

export const readPlaygroundHandoff = (state: unknown): PlaygroundHandoff | null => {
    if (!state || typeof state !== 'object') return null;
    const value = (state as Record<string, unknown>)[KEY];
    return value && typeof value === 'object' ? value as PlaygroundHandoff : null;
};
