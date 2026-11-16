import type { ThreadEvent } from '../../client';
import type { MessageDetail } from '../../transcript/messageDetail';

/** Concise hides cross-thread text and finished-turn notifications, not the
 * thread cards themselves. Human input and actionable cards remain visible. */
export function showManagerMessage(message: ThreadEvent, detail: MessageDetail): boolean {
    if (detail === 'detailed') return true;
    if (message.kind === 'thread_update') {
        try {
            if (JSON.parse(message.payload ?? '{}')?.kind === 'finished') return false;
        } catch {
            // Unknown notifications stay visible rather than losing an action.
        }
    }
    const text = message.kind === 'user' || message.kind === 'assistant';
    return !(text && message.source === 'thread');
}
