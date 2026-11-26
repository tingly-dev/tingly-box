import type { TranscriptBlock } from './model';
import { hasFiles } from './model';

// messageCopyText is the text a message's copy action puts on the clipboard:
// exactly what was written — the user's own words, the assistant's Markdown —
// never the host's context line, attachments, author or time. A message with
// nothing to copy (an empty message, or files sent without a caption) yields
// undefined, so it gets no button. The transcript's other blocks (thinking,
// tool calls and results, system notes) are not messages and have no copy
// action of their own.
export const messageCopyText = (block: TranscriptBlock): string | undefined => {
    if (block.type !== 'user' && block.type !== 'assistant') return undefined;
    const { message } = block;
    // An assistant entry carrying files is the files themselves (send_file);
    // its content is their caption, not a markdown reply. A user entry's
    // attachments are separate from their words, so those still copy.
    if (block.type === 'assistant' && hasFiles(message)) return undefined;
    return message.content.trim() !== '' ? message.content : undefined;
};
