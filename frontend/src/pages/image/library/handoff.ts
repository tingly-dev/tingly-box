// What the library page hands the playground when "use" is picked on a
// piece or an image: router state, read once on arrival. Images travel by id
// (the playground reads them from the store) because router state goes into
// session history, which is no place for megabytes of base64.

export interface PlaygroundHandoff {
    // Replaces the prompt field.
    prompt?: string;
    // Added to what is in the prompt field.
    piece?: string;
    imageIds?: string[];
}

const KEY = 'imageLibraryHandoff';

export const handoffState = (handoff: PlaygroundHandoff) => ({ [KEY]: handoff });

export const readHandoff = (state: unknown): PlaygroundHandoff | null => {
    const value = state && typeof state === 'object' ? (state as Record<string, unknown>)[KEY] : undefined;
    return value && typeof value === 'object' ? value as PlaygroundHandoff : null;
};
