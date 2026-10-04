// The image library (素材库): what the user has chosen to keep, as opposed to
// the run history (what happened, clearable). See .design/image-library.md.
//
// PROTOTYPE: kept in memory (assetStore.ts); the intended home is the image
// output directory on disk, so the library and the folder are one thing.

// Where an image came from — said on its tile, never a separate place.
export type AssetOrigin = 'generated' | 'reference';

export interface ImageAsset {
    id: string;
    src: string;
    name: string;
    origin: AssetOrigin;
    width?: number;
    height?: number;
    createdAt: number;
}

// A reusable piece of prompt text. Inserted by copy: editing it in one
// prompt never changes another (unlike images, which are shared).
export interface PromptSnippet {
    id: string;
    name: string;
    text: string;
    createdAt: number;
}
