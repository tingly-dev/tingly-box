// A prompt kept as a file — a .txt or .md someone iterates on outside the
// browser — opens straight into the prompt field. The rule is one line: a
// text file dropped, pasted or picked anywhere on the playground is the
// prompt; images are images. Everything here is the decision of which is
// which and the reading, so the panel's drop targets share it.

// Files over this are not prompts, whatever their extension.
export const PROMPT_FILE_MAX_BYTES = 256 * 1024;

// Plain-text documents a prompt plausibly lives in: notes, markdown, and the
// structured formats prompt templates get kept in. Their contents are used
// verbatim — a JSON or YAML file is not parsed, it is the prompt as written.
const PROMPT_EXTENSIONS = /\.(txt|text|md|markdown|mdx|rst|org|prompt|json|yaml|yml|toml|csv)$/i;

/**
 * Whether a file is a prompt to open rather than an image to attach. The
 * extension is checked as well as the MIME type because browsers hand `.md`
 * (and most of the others) over with an empty type on several platforms.
 */
export const isPromptFile = (file: Pick<File, 'name' | 'type'>): boolean => (
    file.type.startsWith('text/')
    || file.type === 'application/json'
    || PROMPT_EXTENSIONS.test(file.name)
);

export type PromptFileResult =
    | { ok: true; text: string; name: string }
    | { ok: false; reason: 'too-large' | 'empty' | 'unreadable'; name: string };

/** Reads a prompt file, trimming the trailing newline editors leave behind. */
export const readPromptFile = async (file: File): Promise<PromptFileResult> => {
    if (file.size > PROMPT_FILE_MAX_BYTES) return { ok: false, reason: 'too-large', name: file.name };
    try {
        const text = (await file.text()).replace(/\s+$/, '');
        if (!text.trim()) return { ok: false, reason: 'empty', name: file.name };
        return { ok: true, text, name: file.name };
    } catch {
        return { ok: false, reason: 'unreadable', name: file.name };
    }
};

/** Splits a drop into what becomes the prompt and what becomes an image. */
export const partitionDroppedFiles = (files: FileList | File[]): { prompt: File | null; images: File[] } => {
    const list = Array.from(files);
    return {
        prompt: list.find(isPromptFile) ?? null,
        images: list.filter((file) => file.type.startsWith('image/')),
    };
};
