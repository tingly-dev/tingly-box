import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread } from '../../client';
import { AutoAnswerSelect, autoAnswerOf } from './AutoAnswerSelect';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

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
});
