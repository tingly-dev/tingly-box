import { cleanup, fireEvent, render, screen, waitForElementToBeRemoved, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread } from '../../client';
import { AutoAnswerSelect, autoAnswerOf, customDelayMinutes } from './AutoAnswerSelect';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: Record<string, unknown>) => (o?.duration ? `${key} ${o.duration}` : key) }) }));

afterEach(cleanup);

const thread = (o: Partial<Thread>): Thread => ({
    id: 't', project_id: 'p', title: 'Fix login', task: '', status: 'working', origin: 'user', agent: 'claude-code', seq: 1, cwd: '/work/app',
    commits_ahead: 0, files_changed: 0, uncommitted: false, turns: 0, input_tokens: 0, output_tokens: 0, created_at: '', updated_at: '', ...o,
} as Thread);

describe('AutoAnswerSelect', () => {
    it('defaults to a delay in a project thread and to off in a folder session', () => {
        expect(autoAnswerOf(thread({}))).toBe('delay');
        expect(autoAnswerOf(thread({}), true)).toBe('off');
        expect(autoAnswerOf(thread({ auto_answer: 'now' }), true)).toBe('now');
    });

    it('says the setting in a label that reads on its own, and saves the one picked', () => {
        const onChange = vi.fn();
        render(<AutoAnswerSelect thread={thread({})} onChange={onChange} />);
        const picker = screen.getByRole('combobox', { name: 'autoAnswer.title' });
        expect(picker).toHaveTextContent('autoAnswer.delayLabel');
        fireEvent.mouseDown(picker);
        const list = screen.getByRole('listbox');
        expect(within(list).getByText('autoAnswer.menuTitle')).toBeInTheDocument();
        expect(within(list).getByText('autoAnswer.nowHelp')).toBeInTheDocument();
        fireEvent.click(within(list).getByText('autoAnswer.now'));
        expect(onChange).toHaveBeenCalledWith('now');
    });

    it('reads a custom wait from the thread and shows it as its own entry', () => {
        expect(autoAnswerOf(thread({ auto_answer: 'delay:45' }))).toBe('delay:45');
        expect(autoAnswerOf(thread({ auto_answer: 'delay:0' }))).toBe('delay');
        expect(autoAnswerOf(thread({ auto_answer: 'delay:61' }), true)).toBe('off');
        expect(customDelayMinutes('delay:45')).toBe(45);
        expect(customDelayMinutes('delay')).toBeUndefined();
        render(<AutoAnswerSelect thread={thread({ auto_answer: 'delay:45' })} onChange={vi.fn()} />);
        expect(screen.getByRole('combobox', { name: 'autoAnswer.title' })).toHaveTextContent('autoAnswer.delayCustomLabel 45m');
    });

    it('saves a custom wait as delay:<minutes>, rejects bad input, and cancel changes nothing', async () => {
        const onChange = vi.fn();
        render(<AutoAnswerSelect thread={thread({})} onChange={onChange} />);
        fireEvent.mouseDown(screen.getByRole('combobox', { name: 'autoAnswer.title' }));
        fireEvent.click(within(screen.getByRole('listbox')).getByText('autoAnswer.custom'));
        const input = screen.getByRole('textbox', { name: 'autoAnswer.customInput' });
        fireEvent.change(input, { target: { value: '0' } });
        expect(screen.getByText('autoAnswer.customInvalid')).toBeInTheDocument();
        fireEvent.change(input, { target: { value: '45' } });
        fireEvent.submit(input);
        expect(onChange).toHaveBeenCalledWith('delay:45');
        await waitForElementToBeRemoved(() => screen.queryByRole('dialog'));

        onChange.mockClear();
        fireEvent.mouseDown(screen.getByRole('combobox', { name: 'autoAnswer.title' }));
        fireEvent.click(within(screen.getByRole('listbox')).getByText('autoAnswer.custom'));
        fireEvent.click(screen.getByText('autoAnswer.customCancel'));
        expect(onChange).not.toHaveBeenCalled();
    });

    it('stores the default wait as plain delay when 5 minutes is typed', () => {
        const onChange = vi.fn();
        render(<AutoAnswerSelect thread={thread({ auto_answer: 'now' })} onChange={onChange} />);
        fireEvent.mouseDown(screen.getByRole('combobox', { name: 'autoAnswer.title' }));
        fireEvent.click(within(screen.getByRole('listbox')).getByText('autoAnswer.custom'));
        const input = screen.getByRole('textbox', { name: 'autoAnswer.customInput' });
        fireEvent.change(input, { target: { value: '5' } });
        fireEvent.submit(input);
        expect(onChange).toHaveBeenCalledWith('delay');
    });
});
