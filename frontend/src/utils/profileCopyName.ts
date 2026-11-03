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
