import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PhoneShellContext } from '@tingly/ui/hooks/usePhoneShell';
import { CodeBlock, Markdown } from './Markdown';
import ThreadMarkdown from '../transcript/Markdown';
import { tableToMarkdown } from './tableToMarkdown';

const { writeToClipboard } = vi.hoisted(() => ({ writeToClipboard: vi.fn(async (_text: string) => {}) }));
vi.mock('@tingly/ui/utils/clipboard', async (importOriginal) => ({ ...(await importOriginal<object>()), writeToClipboard }));
vi.mock('react-i18next', () => ({
    useTranslation: () => ({ i18n: { language: 'en' }, t: (key: string, opts?: { defaultValue?: string }) => opts?.defaultValue ?? key }),
}));

beforeEach(() => writeToClipboard.mockClear());
afterEach(cleanup);

const TABLE = '| Name | Qty |\n| :-- | --: |\n| a \\| b | `x\\|y` |\n| **bold** | [l](http://e) |\n';
const EXPECTED = '| Name | Qty |\n| :--- | ---: |\n| a \\| b | `x\\|y` |\n| **bold** | [l](http://e) |';

const table = (html: string) => {
    const el = document.createElement('table');
    el.innerHTML = html;
    return el;
};

describe('tableToMarkdown', () => {
    it('rebuilds a GFM table: alignment row, inline code, bold, links, escaped pipes', () => {
        const el = table(
            '<thead><tr><th style="text-align:left">Name</th><th align="right">Qty</th><th align="center">Unit</th><th>Note</th></tr></thead>' +
            '<tbody><tr><td>a | b</td><td><code>x|y</code></td><td>c</td><td>—</td></tr>' +
            '<tr><td><strong>bold</strong></td><td><a href="http://e">l</a></td><td><s>gone</s></td><td>line<br>break</td></tr></tbody>',
        );
        expect(tableToMarkdown(el)).toBe(
            '| Name | Qty | Unit | Note |\n' +
            '| :--- | ---: | :---: | --- |\n' +
            '| a \\| b | `x\\|y` | c | — |\n' +
            '| **bold** | [l](http://e) | ~~gone~~ | line<br>break |',
        );
    });

    it('returns nothing for a table with no rows', () => {
        expect(tableToMarkdown(table(''))).toBe('');
    });
});

describe('a table copies its own Markdown', () => {
    it('offers one copy button per table and copies the rebuilt Markdown, not the visible text', async () => {
        render(<Markdown conversation text={`Intro paragraph.\n\n${TABLE}\n\nOutro.`} />);
        const buttons = screen.getAllByRole('button', { name: 'Copy table' });
        expect(buttons).toHaveLength(1);
        fireEvent.click(buttons[0]);
        await waitFor(() => expect(writeToClipboard).toHaveBeenLastCalledWith(EXPECTED));
        expect(writeToClipboard.mock.calls.at(-1)?.[0]).not.toContain('Intro paragraph');
        expect(writeToClipboard.mock.calls.at(-1)?.[0]).not.toContain('Outro');
    });

    it('keeps two tables independent', async () => {
        const second = '| A | B |\n| - | - |\n| 1 | 2 |';
        render(<Markdown conversation text={`${TABLE}\n\n${second}`} />);
        const buttons = screen.getAllByRole('button', { name: 'Copy table' });
        expect(buttons).toHaveLength(2);
        fireEvent.click(buttons[1]);
        await waitFor(() => expect(writeToClipboard).toHaveBeenLastCalledWith('| A | B |\n| --- | --- |\n| 1 | 2 |'));
    });

    it('is a keyboard-reachable button that acknowledges a copy with a check', async () => {
        render(<Markdown text={TABLE} />);
        const button = screen.getByRole('button', { name: 'Copy table' }) as HTMLButtonElement;
        expect(button.tagName).toBe('BUTTON');
        button.focus();
        expect(document.activeElement).toBe(button);
        fireEvent.click(button);
        const done = await screen.findByRole('button', { name: 'common.copied' });
        expect(done.getAttribute('data-copy-state')).toBe('copied');
    });

    it('says so when the clipboard refuses', async () => {
        writeToClipboard.mockRejectedValueOnce(new Error('denied'));
        render(<Markdown text={TABLE} />);
        fireEvent.click(screen.getByRole('button', { name: 'Copy table' }));
        const failed = await screen.findByRole('button', { name: 'Could not copy' });
        expect(failed.getAttribute('data-copy-state')).toBe('failed');
    });

    it('works the same through the transcript parser (@ant-design/x-markdown)', async () => {
        render(<ThreadMarkdown content={TABLE} />);
        fireEvent.click(screen.getByRole('button', { name: 'Copy table' }));
        await waitFor(() => expect(writeToClipboard).toHaveBeenLastCalledWith(EXPECTED));
    });
});

describe('code blocks copy the bare code', () => {
    it('copies without the ``` fence or the language tag', async () => {
        render(<Markdown conversation text={'```ts\nconst a = 1;\nconst b = 2;\n```'} />);
        fireEvent.click(screen.getByRole('button', { name: 'common.copy' }));
        await waitFor(() => expect(writeToClipboard).toHaveBeenLastCalledWith('const a = 1;\nconst b = 2;'));
        expect(writeToClipboard.mock.calls.at(-1)?.[0]).not.toContain('ts');
    });

    it('copies the whole block even while the phone folds it to a capped height', async () => {
        const code = Array.from({ length: 200 }, (_, i) => `line ${i}`).join('\n');
        const view = render(<PhoneShellContext.Provider value><CodeBlock code={code} language="text" /></PhoneShellContext.Provider>);
        const pre = view.container.querySelector('pre')!;
        expect(getComputedStyle(pre).maxHeight).toBe('210px');
        fireEvent.click(screen.getByRole('button', { name: 'common.copy' }));
        await waitFor(() => expect(writeToClipboard).toHaveBeenLastCalledWith(code));
    });

    it('shows a failure state when the clipboard refuses', async () => {
        writeToClipboard.mockRejectedValueOnce(new Error('denied'));
        render(<CodeBlock code="x" language="text" />);
        fireEvent.click(screen.getByRole('button', { name: 'common.copy' }));
        const failed = await screen.findByRole('button', { name: 'Could not copy' });
        expect(failed.getAttribute('data-copy-state')).toBe('failed');
    });
});
