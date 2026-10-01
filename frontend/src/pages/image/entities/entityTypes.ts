// Image entities: reusable inputs for image generation — a character or a
// style the user would otherwise re-describe (and re-attach) on every run.
// See .design/image-entity.md.
//
// PROTOTYPE: entities live in an in-memory store seeded with mock data; there
// is no backend yet, and a run does not send an entity's references. The UI
// is here to judge the interaction, not to be wired.

// Two kinds, and only two for now: one pins WHO/WHAT is in the picture, the
// other pins HOW it is drawn. Environments and scenes come later, once these
// two have proven the composition rules.
export type EntityKind = 'character' | 'style';

export const ENTITY_KINDS: EntityKind[] = ['character', 'style'];

export interface ImageEntity {
    id: string;
    kind: EntityKind;
    // What `@` matches. Unique across kinds — one name, one entity.
    name: string;
    // Ordered: when the reference budget is short, earlier images win.
    refs: string[];
    // The standard description an `@name` expands to.
    prompt: string;
    negative?: string;
    uses: number;
    updatedAt: number;
}
