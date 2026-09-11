// The shapes the Image Playground's session is made of, shared by the panel
// that produces them (ImageGenPlaygroundCard) and the overview that lists them
// all (ImageGenGalleryDialog). They live here rather than in the panel so the
// overview doesn't have to import the panel to know what a run is. The logic
// over these shapes lives next door in imageGenSession.ts.

import type { MaskLayers } from '@tingly/vision';

// The region of a reference image the model may repaint, as the request needs
// it and as the editor needs it back. `file` is the alpha PNG that goes on the
// wire beside the image; `previewUrl` tints that region for the thumbnail;
// `layers` are the strokes it was painted from, so a mask that has been applied
// can still be edited rather than only looked at.
export interface ReferenceMask {
    file: File;
    previewUrl: string;
    layers: MaskLayers;
}

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
    // The mask the request carried, if any. It turns the endpoint line into
    // `images/edits · mask`, so someone about to call the API from their own
    // code sees which field produced this result — and it is the whole mask
    // rather than a flag so retrying, or putting the request back in the panel,
    // gets the same region back instead of quietly repainting everything.
    mask?: ReferenceMask;
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

// One tile of the overview. A completed run contributes one tile per image it
// produced — the grid is about images, not about runs — while a run that is
// still going or that failed contributes the one tile that says so.
export type GalleryTile =
    | { key: string; at: number; kind: 'output'; run: GenerationRun; imageIndex: number; src: string }
    | { key: string; at: number; kind: 'pending'; run: GenerationRun }
    | { key: string; at: number; kind: 'failed'; run: GenerationRun }
    | { key: string; at: number; kind: 'import'; item: ImportedImage };
