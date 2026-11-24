// Suggested names for a duplicated profile or Team: the first free
// "<name>-copy" / "<name> copy", then "-copy-2" / " copy 2", ... They mirror
// freeCopyName / freeTeamCopyName in internal/config so the dialog prefills what
// an empty name would get.

const firstFree = (candidate: (n: number) => string, taken: Set<string>): string => {
    for (let n = 1; ; n++) {
        const name = candidate(n);
        if (!taken.has(name)) return name;
    }
};

// A profile name is also a URL alias: letters, digits, '-' and '_' only.
export function suggestProfileCopyName(name: string, existingNames: string[]): string {
    const stem = name.replace(/[^A-Za-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '') || 'profile';
    return firstFree(n => (n === 1 ? `${stem}-copy` : `${stem}-copy-${n}`), new Set(existingNames));
}

// Team names are free text (unique, up to 128 bytes).
export function suggestTeamCopyName(name: string, existingNames: string[]): string {
    const encoder = new TextEncoder();
    const chars = Array.from(name);
    const fit = (suffix: string) => {
        let end = chars.length;
        while (end > 0 && encoder.encode(chars.slice(0, end).join('') + suffix).length > 128) end--;
        return chars.slice(0, end).join('') + suffix;
    };
    return firstFree(n => fit(n === 1 ? ' copy' : ` copy ${n}`), new Set(existingNames));
}
