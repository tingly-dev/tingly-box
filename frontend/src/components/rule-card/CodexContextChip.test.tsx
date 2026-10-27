import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { CodexCompactChip } from './CodexCompactChip';
import { CodexContextChip } from './CodexContextChip';
import ModelRequestHeader from '@/components/ModelRequestHeader';
import { clearCodexCatalogWindowCache } from './useCodexCatalogWindow';

const api = vi.hoisted(() => ({ listScenarioModels: vi.fn(), getAppliedCodexConfig: vi.fn() }));
vi.mock('@/services/api', () => ({ api }));
// Keys with their values, so a note can be checked for the numbers it gives.
vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, values?: Record<string, unknown>) => (values ? `${key} ${JSON.stringify(values)}` : key),
    }),
}));

afterEach(() => {
    cleanup();
    clearCodexCatalogWindowCache();
    api.listScenarioModels.mockReset();
    api.getAppliedCodexConfig.mockReset();
});

const win = (context_window: unknown, max_context_window: unknown, source: unknown) => ({ context_window, max_context_window, source });
// What the server says Auto writes, with the 1M flag off and on (codex_windows).
// DeepSeek: 128K, no larger window.
const DEEPSEEK = { codex_windows: { off: win(128_000, 128_000, 'catalog'), on: win(128_000, 128_000, 'catalog') } };
// A ChatGPT-backed model: 272K, which Codex may raise to 872K; 1M opens 872K.
const SOL = { codex_windows: { off: win(272_000, 872_000, 'catalog'), on: win(872_000, 872_000, 'extended') } };
// Claude Opus 4.6 on a subscription: 200K, and 1M only with the header 1M adds.
const OPUS = { codex_windows: { off: win(200_000, 200_000, 'catalog'), on: win(1_000_000, 1_000_000, 'extended') } };
// A model the catalog doesn't know.
const UNKNOWN = { codex_windows: { off: win(200_000, 200_000, 'default'), on: win(1_000_000, 1_000_000, 'default') } };

/**
 * Serves `entry` as the "luna" model of Codex's model list: its fields (an
 * array for several rules sharing the name), no entry (undefined), a failed
 * read ('failed', answered the way the model API answers one), a rejection,
 * or a promise that never settles.
 */
const serve = (entry: unknown) => {
    if (entry instanceof Error) api.listScenarioModels.mockRejectedValue(entry);
    else if (entry === 'failed') api.listScenarioModels.mockResolvedValue({ success: false, error: 'Failed to fetch' });
    else if (entry === 'never') api.listScenarioModels.mockReturnValue(new Promise(() => {}));
    else api.listScenarioModels.mockResolvedValue({
        object: 'list',
        data: [
            { id: 'other', object: 'model', created: 0, owned_by: 'tingly-box', ...DEEPSEEK },
            ...(entry === undefined ? [] : [entry].flat().map((e) => ({ id: 'luna', object: 'model', created: 0, owned_by: 'tingly-box', ...(e as object) }))),
        ],
    });
};

const chip = (props: { window?: number; context1M?: boolean; onChange?: (n: number) => void }) => (
    <CodexContextChip
        window={props.window ?? 0}
        context1M={props.context1M ?? false}
        requestModel="luna"
        onChange={props.onChange ?? (() => {})}
        buttonSx={{}}
        highlightedButtonSx={{}}
    />
);

// hidden: an open popover hides the rest of the page from assistive tech.
const chipButton = () => screen.getByRole('button', { name: /ruleHeader\.context\.aria/, hidden: true });

const open = async (props: Parameters<typeof chip>[0] & { entry?: unknown }) => {
    serve(props.entry);
    const view = render(chip(props));
    fireEvent.click(chipButton());
    // Let the read settle (a pending one stays pending).
    await act(async () => {});
    return view;
};

const notes = () => Array.from(document.querySelectorAll<HTMLElement>('[data-note]')).map((el) => [el.dataset.note, el.textContent ?? '']);
const noteIds = () => notes().map(([id]) => id);
const note = (id: string) => notes().find(([n]) => n === id)?.[1] ?? '';
const autoCell = () => screen.getAllByRole('radio')[0];
const header = () => screen.getByText('ruleHeader.context.title').parentElement!;

describe('CodexContextChip', () => {
    it('shows the window Auto writes for the request model', async () => {
        await open({ entry: DEEPSEEK });
        expect(api.listScenarioModels).toHaveBeenCalledWith('codex');
        expect(noteIds()).toEqual(['auto', 'apply']);
        expect(note('auto')).toBe('ruleHeader.context.autoCatalog {"value":"128K"}');
        expect(within(autoCell()).getByText('128K')).toBeInTheDocument();
        expect(header()).toHaveTextContent(/^ruleHeader\.context\.title128K$/);
        expect(chipButton()).toHaveAccessibleName(/ruleHeader\.context\.autoValue.*128K/);
    });

    it('with 1M on, shows the larger window the server opens, or the model\'s own when it has none', async () => {
        await open({ context1M: true, entry: SOL });
        expect(noteIds()).toEqual(['auto', 'apply']);
        expect(note('auto')).toBe('ruleHeader.context.auto1M {"value":"872K"}');
        expect(header()).toHaveTextContent(/^ruleHeader\.context\.title872K$/);
        cleanup();

        await open({ context1M: true, entry: OPUS });
        expect(note('auto')).toBe('ruleHeader.context.auto1M {"value":"1M"}');
        cleanup();

        await open({ context1M: true, entry: DEEPSEEK });
        expect(note('auto')).toBe('ruleHeader.context.autoCatalog {"value":"128K"}');
        expect(header()).toHaveTextContent(/^ruleHeader\.context\.title128K$/);
    });

    it('says what turning on 1M gives, as the server works it out', async () => {
        await open({ entry: SOL });
        expect(noteIds()).toEqual(['auto', 'max', 'apply']);
        expect(note('max')).toBe('ruleHeader.context.ceiling {"value":"872K"}');
        cleanup();

        // Codex can't raise Opus 4.6 past 200K itself; the 1M header opens 1M.
        await open({ entry: OPUS });
        expect(note('max')).toBe('ruleHeader.context.ceiling {"value":"1M"}');
        cleanup();

        // No larger window, or only a guess at one: nothing to say.
        await open({ entry: DEEPSEEK });
        expect(noteIds()).toEqual(['auto', 'apply']);
        cleanup();
        await open({ entry: UNKNOWN });
        expect(noteIds()).toEqual(['auto', 'apply']);
        cleanup();

        // With 1M already on there is nothing left to open.
        await open({ context1M: true, entry: SOL });
        expect(noteIds()).toEqual(['auto', 'apply']);
    });

    it('reads the entry that carries the windows; the server already merged rules sharing the name', async () => {
        await open({ entry: [{}, SOL] });
        expect(header()).toHaveTextContent(/^ruleHeader\.context\.title272K$/);
        expect(note('max')).toBe('ruleHeader.context.ceiling {"value":"872K"}');
    });

    it('with its own window, only warns when it is above what the route accepts', async () => {
        await open({ window: 1_000_000, entry: SOL });
        expect(noteIds()).toEqual(['over', 'apply']);
        expect(note('over')).toBe('ruleHeader.context.overCeiling {"value":"1M","max":"872K"}');
        expect(header()).toHaveTextContent(/^ruleHeader\.context\.title1M$/);
        // The Auto cell still says what Auto would be.
        expect(within(autoCell()).getByText('272K')).toBeInTheDocument();
        cleanup();

        await open({ window: 400_000, entry: SOL });
        expect(noteIds()).toEqual(['apply']);
        cleanup();

        // Without the 1M header, Opus 4.6 answers with 200K; with it, 1M.
        await open({ window: 1_000_000, entry: OPUS });
        expect(note('over')).toBe('ruleHeader.context.overCeiling {"value":"1M","max":"200K"}');
        cleanup();
        await open({ window: 1_000_000, context1M: true, entry: OPUS });
        expect(noteIds()).toEqual(['apply']);
        cleanup();

        // An unknown model's 200K is a guess, not a limit to warn about.
        await open({ window: 400_000, entry: UNKNOWN });
        expect(noteIds()).toEqual(['apply']);
    });

    it('marks the window of a model the list does not know as a guess', async () => {
        await open({ entry: UNKNOWN });
        expect(note('auto')).toBe('ruleHeader.context.autoDefault {"value":"200K"}');
        expect(header()).toHaveTextContent(/^ruleHeader\.context\.title200K$/);
        cleanup();
        clearCodexCatalogWindowCache();

        // No entry at all (an inactive rule): what Auto writes for an unknown model.
        await open({ context1M: true, entry: undefined });
        expect(note('auto')).toBe('ruleHeader.context.autoGuess1M');
        expect(header()).toHaveTextContent(/^ruleHeader\.context\.title1M$/);
    });

    it('shows no number until the list is read', async () => {
        await open({ context1M: true, entry: 'never' });
        expect(noteIds()).toEqual(['auto', 'apply']);
        expect(note('auto')).toBe('ruleHeader.context.autoPending');
        expect(within(autoCell()).getByText('…')).toBeInTheDocument();
        expect(header()).toHaveTextContent(/^ruleHeader\.context\.title…$/);
        expect(chipButton()).toHaveAccessibleName('ruleHeader.context.aria {"value":"ruleHeader.context.auto"}');
    });

    it('says when the list could not be read, and treats junk as unknown', async () => {
        for (const entry of ['failed', new Error('offline')]) {
            await open({ entry });
            expect(note('auto'), String(entry)).toBe('ruleHeader.context.autoUnavailable {"value":"200K"}');
            cleanup();
            clearCodexCatalogWindowCache();
        }

        for (const off of [win('272000', 0, 'catalog'), win(-1, 0, 'catalog'), win(1.5, 0, 'catalog'), win(272_000, 0, '1m'), null]) {
            await open({ entry: { codex_windows: { off, on: SOL.codex_windows.on } } });
            expect(note('auto'), JSON.stringify(off)).toBe('ruleHeader.context.autoDefault {"value":"200K"}');
            cleanup();
            clearCodexCatalogWindowCache();
        }
        // The catalog figures alone don't say what Auto writes.
        await open({ entry: { max_input_tokens: 272_000 } });
        expect(note('auto')).toBe('ruleHeader.context.autoDefault {"value":"200K"}');
    });

    it('turning 1M on or off shows the other answer at once, without a new read', async () => {
        const view = await open({ entry: SOL });
        fireEvent.keyDown(screen.getByRole('presentation'), { key: 'Escape' });
        expect(chipButton()).toHaveAccessibleName(/autoValue.*272K/);

        view.rerender(chip({ context1M: true }));
        expect(chipButton()).toHaveAccessibleName(/autoValue.*872K/);
        expect(api.listScenarioModels).toHaveBeenCalledTimes(1);
    });

    it('keeps the last answer on reopen, and in a new chip, while it is read again', async () => {
        await open({ entry: DEEPSEEK });
        cleanup();
        serve('never');
        render(chip({}));
        fireEvent.click(chipButton());
        await act(async () => {});
        expect(api.listScenarioModels).toHaveBeenCalledTimes(2);
        expect(note('auto')).toBe('ruleHeader.context.autoCatalog {"value":"128K"}');
    });

    it('a failed read keeps the last answer', async () => {
        await open({ entry: DEEPSEEK });
        cleanup();
        await open({ entry: 'failed' });
        expect(note('auto')).toBe('ruleHeader.context.autoCatalog {"value":"128K"}');
    });
});

describe('1M menu on a rule header', () => {
    const openMenu = async (entry: unknown, props: { window?: number; codex?: boolean } = {}) => {
        serve(entry);
        render(
            <ModelRequestHeader
                modelName="luna"
                onContext1MToggle={() => {}}
                onContextWindowChange={props.codex === false ? undefined : () => {}}
                contextWindow={props.window ?? 0}
            />,
        );
        fireEvent.click(screen.getByRole('button', { name: /^1M context/ }));
        await act(async () => {});
    };
    const line = (side: 'off' | 'on') => document.querySelector(`[data-codex-window="${side}"]`)?.textContent ?? null;

    it('on a Codex rule, says the window each choice writes', async () => {
        await openMenu(SOL);
        expect(line('off')).toBe('ruleHeader.context1M.codexWindow {"value":"272K"}');
        expect(line('on')).toBe('ruleHeader.context1M.codexWindow {"value":"872K"}');
        cleanup();

        await openMenu(OPUS);
        expect(line('off')).toBe('ruleHeader.context1M.codexWindow {"value":"200K"}');
        expect(line('on')).toBe('ruleHeader.context1M.codexWindow {"value":"1M"}');
    });

    it('marks a guess, and says when Context pins the window', async () => {
        await openMenu(UNKNOWN);
        expect(line('off')).toBe('ruleHeader.context1M.codexWindowGuess {"value":"200K"}');
        expect(line('on')).toBe('ruleHeader.context1M.codexWindowGuess {"value":"1M"}');
        cleanup();

        await openMenu(SOL, { window: 256_000 });
        expect(line('off')).toBe('ruleHeader.context1M.codexWindowPinned {"value":"256K"}');
        expect(line('on')).toBe('ruleHeader.context1M.codexWindowPinned {"value":"256K"}');
    });

    it('waits for the list, and leaves the line out when it cannot be read', async () => {
        await openMenu('never');
        expect(line('on')).toBe('ruleHeader.context1M.codexWindow {"value":"…"}');
        cleanup();

        await openMenu('failed');
        expect(line('off')).toBeNull();
        expect(line('on')).toBeNull();
        cleanup();

        // A pinned window needs no list.
        await openMenu('failed', { window: 256_000 });
        expect(line('on')).toBe('ruleHeader.context1M.codexWindowPinned {"value":"256K"}');
    });

    it('says nothing about Codex on other rules, and does not read its list', async () => {
        await openMenu(SOL, { codex: false });
        expect(line('off')).toBeNull();
        expect(api.listScenarioModels).not.toHaveBeenCalled();
    });
});

describe('CodexCompactChip', () => {
    const compact = (contextWindow = 0, context1M = false) => (
        <CodexCompactChip percent={0} contextWindow={contextWindow} context1M={context1M} requestModel="luna" onChange={() => {}} buttonSx={{}} highlightedButtonSx={{}} />
    );
    const openCompact = async (entry: unknown, contextWindow = 0, context1M = false) => {
        api.getAppliedCodexConfig.mockResolvedValue({ preferences: {} });
        serve(entry);
        render(compact(contextWindow, context1M));
        fireEvent.click(screen.getByRole('button', { name: /ruleHeader\.compact\.aria/ }));
        await act(async () => {});
    };
    const row = (label: string) => screen.getByText(label).parentElement!;

    it('previews against the Auto window', async () => {
        await openCompact(DEEPSEEK);
        expect(row('ruleHeader.compact.window')).toHaveTextContent('128K');
        expect(row('ruleHeader.compact.line')).toHaveTextContent('75% · 96,000 tokens');
    });

    it('with 1M on, previews against the window Auto writes with it', async () => {
        await openCompact(SOL, 0, true);
        expect(row('ruleHeader.compact.window')).toHaveTextContent('872K');
        expect(row('ruleHeader.compact.line')).toHaveTextContent('75% · 654,000 tokens');
    });

    it('shows only the percent until Auto is known, unless the rule has its own window', async () => {
        await openCompact('never');
        expect(row('ruleHeader.compact.window')).toHaveTextContent(/…$/);
        expect(row('ruleHeader.compact.line')).toHaveTextContent(/75%$/);
        cleanup();
        clearCodexCatalogWindowCache();

        // A failed read is no better than a pending one.
        await openCompact(new Error('offline'));
        expect(row('ruleHeader.compact.window')).toHaveTextContent(/…$/);
        expect(row('ruleHeader.compact.remain')).toHaveTextContent(/…$/);
        cleanup();

        await openCompact('never', 256_000);
        expect(row('ruleHeader.compact.window')).toHaveTextContent('256K');
        expect(row('ruleHeader.compact.line')).toHaveTextContent('75% · 192,000 tokens');
    });
});
