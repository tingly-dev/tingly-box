// The image library (素材库): what the user has chosen to keep, as opposed to
// the run history (what happened, clearable). A collection, not a registry:
// using an image copies it into the request or profile, and nothing tracks
// where it went. See .design/image-library.md.
//
// Kept in the browser for now (assetStore.ts → imageWorkspaceDb.ts); the
// intended home is the image output directory on disk, so the library and the
// folder are one thing.

export interface ImageAsset {
    id: string;
    src: string;
    // The bytes behind `src` (a blob: URL), so the library stores the file and
    // not a base64 copy; `src` is rebuilt from it on load.
    blob?: Blob;
    name: string;
    width?: number;
    height?: number;
    createdAt: number;
}

// A reusable piece of prompt text, inserted by copy.
export interface PromptSnippet {
    id: string;
    name: string;
    text: string;
    createdAt: number;
}
