import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ThreadEvent } from '../../client';
import { ThreadUpdateRow } from './ConversationPane';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../Markdown', () => ({ Markdown: ({ text }: { text: string }) => <div data-testid="report">{text}</div> }));

afterEach(cleanup);

const update = (content: string): ThreadEvent => ({
    id: 'u', kind: 'thread_update', content, thread_id: 'main', created_at: '', updated_at: '',
    payload: JSON.stringify({ thread_id: 't', title: 'Fix login', kind: 'finished', status: 'ready' }),
});

describe('ThreadUpdateRow', () => {
    it('keeps the report folded until clicked, and opens the thread from its own link', () => {
        const onOpen = vi.fn();
        render(<ThreadUpdateRow message={update('Fixed it.')} onOpen={onOpen} />);
        expect(screen.getByText('Fix login')).toBeInTheDocument();
        expect(screen.queryByTestId('report')).toBeNull();
        fireEvent.click(screen.getByRole('button', { expanded: false }));
        expect(screen.getByTestId('report').textContent).toBe('Fixed it.');
        expect(onOpen).not.toHaveBeenCalled();
        fireEvent.click(screen.getByRole('button', { name: 'conversation.openThread' }));
        expect(onOpen).toHaveBeenCalledWith('t');
    });
});
