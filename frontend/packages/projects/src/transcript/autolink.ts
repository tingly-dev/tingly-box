import type { XMarkdownProps } from '@ant-design/x-markdown';

// remark-gfm and marked both end a bare URL at ASCII whitespace only, so a URL
// followed by CJK text or full-width punctuation ("https://a.b/c（未合并）")
// swallows what comes after it. These helpers end it where a reader would.

// Punctuation that never belongs to a raw URL: CJK and full-width punctuation,
// the ideographic space, curly quotes, ellipsis and em dash.
const BREAKS = /[—‘-‟…　-〄〈-〠〰〽-〿・＀-￯]/u;
const CJK = /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Hangul}]/u;
// A path segment or query value ends at one of these; in the host, so does a dot.
const SEGMENT_END = '/?#&=+';
const SCHEME = /^https?:\/\//i;

/**
 * Where the URL at the start of `run` (a run of non-space characters) ends.
 * CJK text belongs to the URL only when it starts its segment ("/路径/文件",
 * "?q=中文"); glued to an ASCII segment ("/c然后") it is prose.
 */
export function autolinkEnd(run: string): number {
    let index = SCHEME.exec(run)?.[0].length ?? 0;
    let cjkSegment: boolean | null = null;
    let host = true;
    for (const ch of run.slice(index)) {
        if (BREAKS.test(ch)) return index;
        if ('/?#'.includes(ch)) host = false;
        if (SEGMENT_END.includes(ch) || (host && ch === '.')) {
            cjkSegment = null;
        } else if (cjkSegment === null) {
            cjkSegment = CJK.test(ch);
        } else if (!cjkSegment && CJK.test(ch)) {
            return index;
        }
        index += ch.length;
    }
    return run.length;
}

const count = (s: string, ch: string) => s.split(ch).length - 1;

/** Drops the sentence punctuation GFM leaves off the end of an autolink. */
export function trimUrlTail(url: string): string {
    let end = url.length;
    for (;;) {
        const tail = url[end - 1];
        if (tail && '?!.,:*_~'.includes(tail)) end--;
        else if (tail === ')' && count(url.slice(0, end), ')') > count(url.slice(0, end), '(')) end--;
        else return url.slice(0, end);
    }
}

interface MdNode {
    type: string;
    value?: string;
    children?: MdNode[];
    position?: { start: { offset?: number }; end: { offset?: number } };
}

// A link remark-gfm made from bare text: its source is exactly its text, so
// "[url](url)" and "<url>" (which carry markup) are not it.
const bareUrl = (node: MdNode, source: string): { at: number; text: string } | null => {
    const [child] = node.children ?? [];
    if (node.type !== 'link' || node.children?.length !== 1 || child?.type !== 'text' || !child.value) return null;
    const { start, end } = node.position ?? {};
    if (start?.offset === undefined || end?.offset === undefined) return null;
    if (source.slice(start.offset, end.offset) !== child.value) return null;
    return /^(https?:\/\/|www\.)/i.test(child.value) ? { at: start.offset, text: child.value } : null;
};

interface Edit { at: number; length: number; to: string }

function collect(node: MdNode, source: string, edits: Edit[]) {
    for (const child of node.children ?? []) {
        const bare = bareUrl(child, source);
        if (!bare) {
            if (child.type !== 'link') collect(child, source, edits);
            continue;
        }
        const url = trimUrlTail(bare.text.slice(0, autolinkEnd(bare.text)));
        if (url.length === bare.text.length || /[<>]/.test(url)) continue;
        // "<url>" is an autolink with an end of its own; "www." has no scheme, so it needs a link written out.
        const to = SCHEME.test(url) ? `<${url}>` : `[${url.replace(/[!-/:-@[-`{-~]/g, '\\$&')}](<http://${url.replace(/[\\&]/g, '\\$&')}>)`;
        edits.push({ at: bare.at, length: url.length, to });
    }
}

// An autolink that ran into what followed it also swallowed the markup
// around it: in "**https://a.b/c**，好" the closing ** is part of the "URL",
// so the bold never closes. Rewriting the URL as "<url>" and parsing again
// lets the rest of the line be read as it was written.
const ROUNDS = 8;

/**
 * remark plugin, after remark-gfm: ends the links it autolinks at CJK text and
 * full-width punctuation. It wraps the parser, so it must come last.
 */
export function remarkAutolinkBoundary(this: { parser?: unknown }) {
    const parse = this.parser as (doc: string, file: unknown) => MdNode;
    this.parser = (doc: string, file: unknown) => {
        let source = doc;
        let tree = parse(source, file);
        for (let round = 0; round < ROUNDS; round++) {
            const edits: Edit[] = [];
            collect(tree, source, edits);
            if (edits.length === 0) break;
            for (const { at, length, to } of edits.sort((x, y) => y.at - x.at)) source = source.slice(0, at) + to + source.slice(at + length);
            tree = parse(source, file);
        }
        return tree;
    };
}

/** marked config for x-markdown: the same boundary for its bare-URL tokenizer. */
export const autolinkBoundaryConfig: NonNullable<XMarkdownProps['config']> = {
    tokenizer: {
        url(src) {
            const run = /^(?:https?:\/\/|www\.)[^\s<]*/i.exec(src)?.[0];
            if (!run) return false;
            const end = autolinkEnd(run);
            if (end === run.length) return false;
            // The default tokenizer on the shortened source: it still trims
            // trailing punctuation and unbalanced parentheses itself.
            return Object.getPrototypeOf(this).url.call(this, src.slice(0, end)) || undefined;
        },
    },
};
