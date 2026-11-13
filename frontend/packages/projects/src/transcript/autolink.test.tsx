import { cleanup, render } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Markdown } from '../components/Markdown';
import TranscriptMarkdown from './Markdown';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));
afterEach(cleanup);

// Both renderers autolink bare URLs on their own (remark-gfm / marked), and
// neither stops at CJK text or full-width punctuation. A URL ends where the
// writer's URL ends; the rest of the sentence is still text.
const renderers = {
    'workspace Markdown (remark)': (text: string) => render(<Markdown text={text} />).container,
    'workspace Markdown, conversation + safe (remark)': (text: string) => render(<Markdown text={text} conversation safe />).container,
    'transcript Markdown (x-markdown)': (text: string) => render(<TranscriptMarkdown content={text} />).container,
};

// href as the browser would show it: react-markdown percent-encodes, x-markdown doesn't.
const hrefs = (container: HTMLElement) => [...container.querySelectorAll('a')].map((a) => decodeURI(a.getAttribute('href') ?? ''));
const linkTexts = (container: HTMLElement) => [...container.querySelectorAll('a')].map((a) => a.textContent);

type Case = [name: string, text: string, links: string[]];
const cases: Case[] = [
    ['CJK full-width parens', 'see https://github.com/x/pull/2544（未合并）', ['https://github.com/x/pull/2544']],
    ['full-width comma then CJK', 'see https://a.b/c，然后继续', ['https://a.b/c']],
    ['ideographic full stop', '见 https://a.b/c。下一句', ['https://a.b/c']],
    ['ideographic comma between two URLs', 'https://a.b/c、https://a.b/d', ['https://a.b/c', 'https://a.b/d']],
    ['full-width semicolon between two URLs', 'https://a.b/c；https://a.b/d', ['https://a.b/c', 'https://a.b/d']],
    ['full-width exclamation', 'https://a.b/c！', ['https://a.b/c']],
    ['full-width question and colon', 'https://a.b/c？ https://a.b/d：好', ['https://a.b/c', 'https://a.b/d']],
    ['CJK straight after an ASCII path', 'https://a.b/c中文', ['https://a.b/c']],
    ['CJK straight after the host', 'https://github.com中文', ['https://github.com']],
    ['CJK after a file extension', '打开 https://a.b/index.html然后刷新', ['https://a.b/index.html']],
    ['CJK after a query value', '打开 https://a.b/p?q=abc然后', ['https://a.b/p?q=abc']],
    ['Japanese kana after a URL', 'https://a.b/cです', ['https://a.b/c']],
    ['Korean after a URL', 'https://a.b/c입니다', ['https://a.b/c']],
    ['URL wrapped in full-width parens', '（https://a.b/c）ok', ['https://a.b/c']],
    ['URL wrapped in corner brackets', '「https://a.b/c」ok', ['https://a.b/c']],
    ['URL wrapped in lenticular brackets', '【https://a.b/c】ok', ['https://a.b/c']],
    ['URL wrapped in book-title marks', '《https://a.b/c》ok', ['https://a.b/c']],
    ['URL in curly quotes', '“https://a.b/c”和‘https://a.b/d’', ['https://a.b/c', 'https://a.b/d']],
    ['URL in bold', '**https://a.b/c** next', ['https://a.b/c']],
    ['URL in bold followed by CJK punctuation', '**https://a.b/c**，然后', ['https://a.b/c']],
    ['URL in bold with CJK parens inside', '**https://a.b/c（未合并）**', ['https://a.b/c']],
    ['URL in bold with CJK parens outside', '**https://a.b/c**（未合并）', ['https://a.b/c']],
    ['URL in italic with CJK comma', '*https://a.b/c，然后*', ['https://a.b/c']],
    ['URL in a list item', '- https://a.b/c（见上）\n- https://a.b/d。', ['https://a.b/c', 'https://a.b/d']],
    ['URL in a table cell', '| a | b |\n| - | - |\n| https://a.b/c（x） | 好 |', ['https://a.b/c']],
    ['URL after a CJK word with no space', '链接https://a.b/c，好', ['https://a.b/c']],
    ['full-width punctuation after an ASCII-trimmed tail', 'https://a.b/c.。', ['https://a.b/c']],
    ['ASCII punctuation run', 'go to https://a.b/c, then https://a.b/d. Then https://a.b/e: x', ['https://a.b/c', 'https://a.b/d', 'https://a.b/e']],
    ['ASCII parens around', '(https://a.b/c) ok', ['https://a.b/c']],
    ['ASCII dot after CJK cut', 'https://a.b/c.然后', ['https://a.b/c']],
    ['www literal then CJK', 'www.example.com，好', ['http://www.example.com']],
    ['http (not https)', 'http://a.b/c）', ['http://a.b/c']],

    ['balanced parens in a path', 'https://en.wikipedia.org/wiki/Foo_(bar) ok', ['https://en.wikipedia.org/wiki/Foo_(bar)']],
    ['balanced parens in a path, then CJK parens', 'https://en.wikipedia.org/wiki/Foo_(bar)（见）', ['https://en.wikipedia.org/wiki/Foo_(bar)']],
    ['query string and fragment', 'https://a.b/p?q=1&r=2#frag, ok', ['https://a.b/p?q=1&r=2#frag']],
    ['query string and fragment, then CJK', 'https://a.b/p?q=1&r=2#frag，好', ['https://a.b/p?q=1&r=2#frag']],
    ['non-ASCII path', 'https://a.b/路径/文件 ok', ['https://a.b/路径/文件']],
    ['non-ASCII path, then CJK comma', 'https://a.b/路径/文件，好', ['https://a.b/路径/文件']],
    ['wikipedia style CJK title', 'https://zh.wikipedia.org/wiki/中文维基百科 ok', ['https://zh.wikipedia.org/wiki/中文维基百科']],
    ['non-ASCII query value', 'https://a.b/s?q=中文&x=1 ok', ['https://a.b/s?q=中文&x=1']],
    ['non-ASCII fragment', 'https://a.b/p#章节 ok', ['https://a.b/p#章节']],
    ['port, userinfo-free', 'https://a.b:8080/c?d=e，好', ['https://a.b:8080/c?d=e']],
    ['nothing to cut', 'plain https://a.b/c/d.html here', ['https://a.b/c/d.html']],
];

describe.each(Object.entries(renderers))('autolink boundary: %s', (_, renderIt) => {
    it.each(cases)('%s', (_name, text, links) => {
        const container = renderIt(text);
        expect(hrefs(container)).toEqual(links);
        // the link text is the URL, and the words that followed it are still on the page
        if (!text.includes('www.')) expect(linkTexts(container).map((t) => decodeURI(t ?? ''))).toEqual(links);
        const source = text.replace(/\*+/g, '').replace(/^- /gm, '').replace(/\|[ -]+\|[ -]+\|\n/, '');
        const shown = (container.textContent ?? '').replace(/\s+/g, '');
        for (const word of source.split(/[\s|]+/).filter(Boolean)) expect(shown).toContain(word.replace(/\s+/g, ''));
    });

    it('leaves explicit links alone', () => {
        const c = renderIt('[官网](https://a.b/c) 和 <https://a.b/d> 和 [https://a.b/e，x](https://a.b/e)');
        expect(hrefs(c)).toEqual(['https://a.b/c', 'https://a.b/d', 'https://a.b/e']);
    });

    it('does not link URLs in code', () => {
        const c = renderIt('`https://a.b/c，好` 和\n\n```\nhttps://a.b/d，好\n```');
        expect(hrefs(c)).toEqual([]);
    });
});

describe.each(Object.entries(renderers))('bold survives the cut: %s', (_, renderIt) => {
    it.each([
        '**https://a.b/c**（未合并）',
        '**https://a.b/c**，然后',
        '**https://a.b/c（未合并）**',
        '**https://a.b/c** next',
    ])('%s', (text) => {
        const c = renderIt(text);
        expect(c.querySelector('strong')).not.toBeNull();
        expect(c.textContent).not.toContain('*');
    });
});
