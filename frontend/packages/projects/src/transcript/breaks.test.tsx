import { cleanup, render } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Markdown } from '../components/Markdown';
import TranscriptMarkdown from './Markdown';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));
afterEach(cleanup);

// One line the author wrote is one line on screen: a single newline is a hard
// break (<br>), in both renderers, so the thread and the workspace agree.
const renderers = {
    'workspace Markdown (remark)': (text: string) => render(<Markdown text={text} />).container,
    'workspace Markdown, conversation + safe (remark)': (text: string) => render(<Markdown text={text} conversation safe />).container,
    'transcript Markdown (x-markdown)': (text: string) => render(<TranscriptMarkdown content={text} />).container,
};

const count = (c: HTMLElement, selector: string) => c.querySelectorAll(selector).length;

describe.each(Object.entries(renderers))('line breaks: %s', (_, renderIt) => {
    it('renders a single newline as one <br>', () => {
        const c = renderIt('first\nsecond\nthird');
        expect(count(c, 'p')).toBe(1);
        expect(count(c, 'br')).toBe(2);
        expect(c.querySelector('p')?.innerHTML).toMatch(/first<br>\s*second<br>\s*third/);
    });

    it('renders a single newline in Chinese text as one <br>', () => {
        const c = renderIt('第一行\n第二行，含有标点。\n第三行');
        expect(count(c, 'p')).toBe(1);
        expect(count(c, 'br')).toBe(2);
    });

    it('renders two trailing spaces as one <br>, not two', () => {
        const c = renderIt('first  \nsecond');
        expect(count(c, 'br')).toBe(1);
    });

    it('renders a trailing backslash as one <br>, not two, and drops the backslash', () => {
        const c = renderIt('first\\\nsecond');
        expect(count(c, 'br')).toBe(1);
        expect(c.textContent).not.toContain('\\');
    });

    it('keeps blank-line paragraphs as paragraphs, without <br>', () => {
        const c = renderIt('first\n\nsecond\n\nthird');
        expect(count(c, 'p')).toBe(3);
        expect(count(c, 'br')).toBe(0);
    });

    it('adds no <br> for a trailing newline', () => {
        expect(count(renderIt('first\nsecond\n'), 'br')).toBe(1);
        expect(count(renderIt('only\n'), 'br')).toBe(0);
    });

    it('breaks inside a list item, and leaves the items themselves alone', () => {
        const c = renderIt('- one\n  continued\n- two\n\n1. a\n   b\n2. c');
        expect(count(c, 'li')).toBe(4);
        expect(count(c, 'li br')).toBe(2);
        expect(count(c, 'br')).toBe(2);
    });

    it('breaks inside a blockquote', () => {
        const c = renderIt('> quoted one\n> quoted two\n\nafter');
        expect(count(c, 'blockquote br')).toBe(1);
        expect(count(c, 'br')).toBe(1);
    });

    it('keeps a table a table: rows are rows, cells get no <br>', () => {
        const c = renderIt('| a | b |\n| - | - |\n| 1 | 2 |\n| 3 | 4 |');
        expect(count(c, 'table')).toBe(1);
        expect(count(c, 'tr')).toBe(3);
        expect(count(c, 'br')).toBe(0);
    });

    it('breaks the text around a table without breaking the table', () => {
        const c = renderIt('before\nmid\n\n| a | b |\n| - | - |\n| 1 | 2 |\n\nafter');
        expect(count(c, 'table')).toBe(1);
        expect(count(c, 'table br')).toBe(0);
        expect(count(c, 'br')).toBe(1);
    });

    it('leaves a fenced code block untouched', () => {
        const c = renderIt('```ts\nline one\nline two\nline three\n```');
        expect(count(c, 'br')).toBe(0);
        expect(c.textContent).toContain('line one');
        expect(c.textContent).toContain('line three');
    });

    it('leaves a fenced block inside a list untouched, and breaks the prose around it', () => {
        const c = renderIt('before\nafter\n\n```\na\nb\n```\n\nend\nend2');
        expect(count(c, 'br')).toBe(2);
        expect(count(c, 'pre br, code br')).toBe(0);
    });

    it('breaks around inline markup', () => {
        const c = renderIt('**bold**\n*it* and `code`\nlast');
        expect(count(c, 'br')).toBe(2);
        expect(count(c, 'strong')).toBe(1);
    });

    it('does not regress the autolink boundary across lines', () => {
        const c = renderIt('see https://a.b/c，然后\nnext https://a.b/d。\n结束');
        expect([...c.querySelectorAll('a')].map((a) => decodeURI(a.getAttribute('href') ?? ''))).toEqual(['https://a.b/c', 'https://a.b/d']);
        expect(count(c, 'br')).toBe(2);
    });

    it('keeps a URL on its own line as its own link', () => {
        const c = renderIt('https://a.b/c\nhttps://a.b/d');
        expect(count(c, 'a')).toBe(2);
        expect(count(c, 'br')).toBe(1);
    });
});
