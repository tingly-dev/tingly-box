import type { ReferenceImage } from '../components/ImageGenReferenceImages';
import type { Quality } from '../components/ImageGenPlayground.types';

// An image profile pins one piece of ongoing work: the reference images
// (picked by the user, within the run's limit), the description every run
// starts from, and the request settings. Its page is the playground with all
// of that already in place — only what changes from run to run is typed.
// See .design/image-profile.md.
//
// PROTOTYPE: profiles live in memory (imageProfileStore.ts); no backend yet.
export interface ImageProfile {
    id: string;
    name: string;
    refs: ReferenceImage[];
    basePrompt: string;
    // '' = the first available image model.
    model: string;
    size: string;
    quality: Quality;
    count: number;
    updatedAt: number;
}
