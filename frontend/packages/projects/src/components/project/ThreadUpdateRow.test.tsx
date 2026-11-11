import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ThreadUpdateRow } from './ConversationPane';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../Markdown', () => ({ Markdown: ({ text }: { text: string }) => <div data-testid="report">{text}</div> }));

afterEach(cleanup);

// The row reads the transcript entry the adapter emits: the payload arrives
// parsed and the detail context defaults to detailed.
const update = (content: string) => ({
    id: 'u', kind: 'thread_update', content, timestamp: '',
    payload: { thread_id: 't', title: 'Fix login', kind: 'finished', status: 'ready' },
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
