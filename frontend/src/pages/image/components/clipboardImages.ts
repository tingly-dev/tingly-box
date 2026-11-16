// The image pages' two ways of taking a picture from the clipboard: a paste
// event (always works), and a button that asks the clipboard directly
// (Chromium-family browsers grant it after a prompt; elsewhere it rejects).

// Image files carried by a paste event; empty when it pasted only text.
export const pastedImageFiles = (event: React.ClipboardEvent): File[] => (
    Array.from(event.clipboardData?.items ?? [])
        .filter((item) => item.kind === 'file')
        .map((item) => item.getAsFile())
        .filter((file): file is File => file !== null && file.type.startsWith('image/'))
);

// Every image on the clipboard; empty when there is none or the browser will
// not hand the clipboard to a click, so callers show the Ctrl+V hint either way.
export const readClipboardImages = async (): Promise<File[]> => {
    try {
        const files: File[] = [];
        for (const item of await navigator.clipboard.read()) {
            const type = item.types.find((candidate) => candidate.startsWith('image/'));
            if (!type) continue;
            const blob = await item.getType(type);
            files.push(new File([blob], `pasted-${Date.now()}.${type.split('/')[1] ?? 'png'}`, { type }));
        }
        return files;
    } catch {
        return [];
    }
};
