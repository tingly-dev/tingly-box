import type { XMarkdownProps } from '@ant-design/x-markdown';
import remarkBreaks from 'remark-breaks';

// Line breaks are hard breaks: the people and agents writing here type one line
// per line (notes, reports, chat), so a single newline renders as <br>, where
// CommonMark would fold it into a space. Blank lines still split paragraphs,
// and code (fenced or inline) keeps its own newlines. Every Markdown surface
// takes this from here, so the thread and the workspace agree.

/** remark plugin (react-markdown): a newline in text becomes a hard break. */
export { remarkBreaks };

/** marked config (x-markdown): the same rule. */
export const breaksConfig: NonNullable<XMarkdownProps['config']> = { breaks: true };
