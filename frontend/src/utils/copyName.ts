// Suggests a free "<name>-copy" name for a duplicated profile. Mirrors the
// backend's default (internal/config/profile_duplicate.go) so the dialog
// prefills what an empty name would get.
export function suggestProfileCopyName(name: string, existingNames: string[]): string {
    const stem = name.replace(/[^A-Za-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '') || 'profile';
    const taken = new Set(existingNames);
    let candidate = `${stem}-copy`;
    for (let n = 2; taken.has(candidate); n++) {
        candidate = `${stem}-copy-${n}`;
    }
    return candidate;
}

// Suggests a free "<name> copy" name for a duplicated Team. Team names are
// free text (unique, up to 128 bytes), unlike profile names. Mirrors
// freeTeamCopyName in internal/config/team.go.
export function suggestTeamCopyName(name: string, existingNames: string[]): string {
    const taken = new Set(existingNames);
    const encoder = new TextEncoder();
    const fit = (suffix: string) => {
        let stem = name;
        while (encoder.encode(stem + suffix).length > 128 && stem.length > 0) stem = Array.from(stem).slice(0, -1).join('');
        return stem + suffix;
    };
    let candidate = fit(' copy');
    for (let n = 2; taken.has(candidate); n++) candidate = fit(` copy ${n}`);
    return candidate;
}
