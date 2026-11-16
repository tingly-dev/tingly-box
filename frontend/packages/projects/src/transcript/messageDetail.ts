import type { TranscriptBlock } from './model';

export type MessageDetail = 'detailed' | 'concise';

/** Filter only after calls, results and requests have been paired. Keep block
 * indexes stable for trajectory anchors, and never discard a request/decision. */
export function conciseBlock(block: TranscriptBlock): TranscriptBlock | null {
    if (block.type !== 'activity') return block;
    const steps = block.steps.filter((step) => step.type !== 'tool');
    return steps.length ? { ...block, steps } : null;
}
