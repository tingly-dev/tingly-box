import {cleanup, render, screen} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {createMemoryDraftStorage} from './context';
import Transcript from './Transcript';
import type {TranscriptEntry} from './model';
import {buildTranscript} from './model';

vi.mock('react-i18next', () => ({useTranslation: () => ({i18n: {language: 'en'}, t: (_key: string, opts: {defaultValue: string}) => opts.defaultValue})}));

const at = (d: number, h: number, m = 0) => new Date(2026, 10, d, h, m).toISOString();
const say = (role: 'user' | 'assistant', content: string, timestamp: string): TranscriptEntry => ({role, content, timestamp});

beforeEach(() => {vi.useFakeTimers({now: new Date(2026, 10, 19, 15, 0), toFake: ['Date']});});
afterEach(() => {
    cleanup();
    vi.useRealTimers();
});

const entries = [
    say('user', 'first', at(17, 9, 0)),
    say('assistant', 'second', at(17, 9, 10)),
    say('user', 'third', at(18, 14, 0)),
    say('assistant', 'fourth', at(18, 14, 5)),
    say('user', 'fifth', at(18, 16, 30)),
    say('assistant', 'sixth', at(19, 8, 0)),
];
const view = (list: TranscriptEntry[], extra: Partial<Parameters<typeof Transcript>[0]> = {}) =>
    render(<Transcript blocks={buildTranscript(list)} working={false} onRespond={vi.fn()} drafts={createMemoryDraftStorage()} {...extra}/>);
const dividers = (root: HTMLElement) => [...root.querySelectorAll('[data-time-divider]')].map((el) => ({kind: el.getAttribute('data-time-divider'), text: el.textContent?.replace(/\s/g, ' '), label: el.getAttribute('aria-label')}));

describe('time dividers', () => {
    it('separates days and long silences, not messages', () => {
        const {container} = view(entries);
        expect(dividers(container).map((d) => [d.kind, d.text])).toEqual([
            ['day', 'Tue, Nov 17 · 9:00 AM'],
            ['day', 'Yesterday · 2:00 PM'],
            ['hour', '4:30 PM'],
            ['day', 'Today · 8:00 AM'],
        ]);
    });

    it('names each divider for assistive technology by its full date and time', () => {
        view(entries);
        const separators = screen.getAllByRole('separator');
        expect(separators).toHaveLength(4);
        expect(separators[1]).toHaveAccessibleName(/Wednesday, November 18, 2026/);
    });

    it('puts a divider above the message it dates', () => {
        view(entries);
        const divider = screen.getAllByRole('separator')[1];
        expect(divider.compareDocumentPosition(screen.getByText('second')) & Node.DOCUMENT_POSITION_PRECEDING).toBeTruthy();
        expect(divider.compareDocumentPosition(screen.getByText('third')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });

    it('keeps every divider where it was when events stream in at the end', () => {
        const {container, rerender} = view(entries);
        const before = dividers(container);
        const grown = [...entries, say('assistant', 'seventh', at(19, 8, 20)), say('user', 'eighth', at(19, 9, 0)), say('assistant', 'ninth', at(19, 11, 0))];
        rerender(<Transcript blocks={buildTranscript(grown)} working={false} onRespond={vi.fn()} drafts={createMemoryDraftStorage()}/>);
        const after = dividers(container);
        expect(after.slice(0, before.length)).toEqual(before);
        expect(after).toHaveLength(before.length + 1);
        expect(after.at(-1)).toMatchObject({kind: 'hour'});
    });

    it('draws none over a block the host does not draw', () => {
        const {container} = view([
            say('user', 'start', at(19, 8, 0)),
            {kind: 'custom_card', content: 'x', timestamp: at(19, 12, 0)},
            say('assistant', 'end', at(19, 12, 1)),
        ], {renderCustom: (m) => (m.kind === 'custom_card' ? null : undefined)});
        // The hidden block would have drawn a divider for 12:00; the reply at 12:01 gets it.
        expect(dividers(container).map((d) => d.kind)).toEqual(['day', 'hour']);
        expect(screen.getAllByRole('separator')[1].previousElementSibling).toBe(screen.getByText('start').closest('[data-block]'));
    });

    it('shows the exact time of a message while hovered, out of the reading order', () => {
        const {container} = view(entries);
        const stamps = container.querySelectorAll('time.message-stamp');
        expect(stamps).toHaveLength(6);
        expect(stamps[0]).toHaveAttribute('aria-hidden', 'true');
        expect(stamps[0]).toHaveTextContent(/Nov 17, 2026, 9:00:00/);
    });
});
