// The shapes the Image Playground's session is made of, shared by the panel
// that produces them (ImageGenPlaygroundCard) and the overview that lists them
// all (ImageGenGalleryDialog). They live here rather than in the panel so the
// overview doesn't have to import the panel to know what a run is.

// Which gateway endpoint a run went through. Not a user choice: derived from
// whether the run had reference images. Shown on the history card so API users
// learn which endpoint does what they just did.
export type Endpoint = 'generations' | 'edits';

export type Quality = 'auto' | 'high' | 'medium' | 'low' | 'standard';

export interface ImageResult {
    url?: string;
    b64_json?: string;
}

// An image the user brought in to work on rather than to generate from: it
// lands in the results panel, not in the request.
export interface ImportedImage {
    id: string;
    src: string;
    name: string;
    bytes: number;
    width?: number;
    height?: number;
    createdAt: number;
}

export interface GenerationRun {
    id: string;
    endpoint: Endpoint;
    createdAt: number;
    prompt: string;
    model: string;
    size: string;
    quality: Quality;
    images: ImageResult[];
    // Data URLs of the reference images a run was built from, kept for display
    // alongside the output — the "what did I ask for" half of the history card
    // (only set when the run went through `edits`).
    sourceImages?: string[];
    // How many images the run asked for — kept so a failed run can be retried
    // with exactly the request it made.
    count?: number;
    status?: 'pending' | 'completed' | 'failed';
    // Why a failed run failed, shown on its card. A run that fails stays in the
    // strip: silently removing it leaves the user with a toast that has already
    // gone and no record of what was asked.
    error?: string;
}

export interface SelectedImage {
    src: string;
    prompt: string;
    model: string;
    size: string;
    quality: Quality;
    index: number;
    // Where this image came from — same lightbox, different framing. A run's
    // output and its `source` originals carry a prompt and a model; a
    // `reference` waiting in the request and an `import` brought in to work on
    // carry neither, so those two are titled by their file instead.
    kind: 'output' | 'source' | 'reference' | 'import';
    // Set for `reference` and `import`: what to call this image and what it is.
    label?: string;
    caption?: string;
    // The run this image belongs to (`output` / `source`), so the lightbox can
    // offer the same "put this request back in the panel" action the card does.
    runId?: string;
}

export const formatBytes = (bytes: number): string => (bytes >= 1024 * 1024
    ? `${(bytes / (1024 * 1024)).toFixed(1)} MB`
    : `${Math.max(1, Math.round(bytes / 1024))} KB`);

// What an API result renders from: the URL when the provider returned one,
// otherwise the inline base64 it sent instead.
export const resultSrc = (image: ImageResult): string => (image.url
    || (image.b64_json ? `data:image/png;base64,${image.b64_json}` : ''));
