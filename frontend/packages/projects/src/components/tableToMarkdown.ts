// Rebuilds a GFM Markdown table from its rendered DOM. Neither parser
// (react-markdown here, @ant-design/x-markdown in the transcript) hands a
// component the table's source text, so copying "the original markdown"
// means rebuilding it from the cells. The rules are deliberately small and
// follow what GFM itself parses:
//   - one line per row, cells joined by " | " and wrapped in pipes;
//   - the second line is the alignment row, read from each header cell's own
//     text-align (react-markdown) or align (marked/x-markdown) — the source's
//     alignment is not preserved anywhere else;
//   - inline markers are restored for the inline elements a cell can hold:
//     code, strong/b, em/i, del/s, links and images; a line break is <br>;
//   - a pipe in text, links or inline code is escaped as \| (GFM requires it
//     even inside code spans, since cells split before inline parsing).
// Limits worth knowing: a link URL containing ")" or whitespace is not
// re-escaped (GFM would need <> or backslashes); raw HTML in a cell was never
// rendered, so it cannot come back; and alignment falls back to no colon when
// the DOM carries none.
const escapeCell = (text: string) => text.replace(/\|/g, '\\|').replace(/\r?\n/g, ' ');

const inlineCode = (text: string) => {
    const runs = text.match(/`+/g);
    const fence = '`'.repeat((runs ? Math.max(...runs.map((run) => run.length)) : 0) + 1);
    // A backtick or space at either edge needs padding so the fence stays
    // distinguishable from the content.
    const pad = /^[` ]|[` ]$/.test(text) ? ' ' : '';
    return `${fence}${pad}${text}${pad}${fence}`;
};

const inline = (node: Node): string => {
    if (node.nodeType === 3) return escapeCell(node.nodeValue ?? '');
    if (node.nodeType !== 1) return '';
    const el = node as Element;
    const children = () => Array.from(el.childNodes).map(inline).join('');
    switch (el.tagName.toLowerCase()) {
        case 'br': return '<br>';
        case 'code': return inlineCode(escapeCell(el.textContent ?? ''));
        case 'strong': case 'b': return `**${children()}**`;
        case 'em': case 'i': return `*${children()}*`;
        case 'del': case 's': return `~~${children()}~~`;
        case 'a': {
            const href = escapeCell(el.getAttribute('href') ?? '');
            return href ? `[${children()}](${href})` : children();
        }
        case 'img': {
            const alt = escapeCell(el.getAttribute('alt') ?? '');
            const src = escapeCell(el.getAttribute('src') ?? '');
            return src ? `![${alt}](${src})` : alt;
        }
        default: return children();
    }
};

const alignment = (cell: Element): '' | 'left' | 'center' | 'right' => {
    const raw = ((cell as HTMLElement).style?.textAlign || cell.getAttribute('align') || '').toLowerCase();
    return raw === 'center' || raw === 'right' || raw === 'left' ? raw : '';
};

const rule = (align: string) => align === 'center' ? ':---:' : align === 'right' ? '---:' : align === 'left' ? ':---' : '---';

export function tableToMarkdown(table: HTMLTableElement): string {
    const rows = Array.from(table.rows);
    if (rows.length === 0) return '';
    const line = (cells: Element[]) => `| ${cells.map((cell) => inline(cell).trim()).join(' | ')} |`;
    const header = Array.from(rows[0].cells);
    return [
        line(header),
        `| ${header.map((cell) => rule(alignment(cell))).join(' | ')} |`,
        ...rows.slice(1).map((row) => line(Array.from(row.cells))),
    ].join('\n');
}
