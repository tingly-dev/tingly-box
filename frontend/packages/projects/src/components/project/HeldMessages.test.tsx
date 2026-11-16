import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { ThreadEvent } from '../../client';
import { HeldMessages, heldMessages } from './HeldMessages';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string) => ({ 'queue.edit': 'Edit', 'queue.remove': 'Remove', 'queue.heading': 'Sends after this turn' })[key] ?? key }),
}));

const ev = (id: string, kind: string, content: string, payload?: string): ThreadEvent => ({
    id, thread_id: 't', kind, content, payload, created_at: '2026-10-24T00:00:00Z', updated_at: '2026-10-24T00:00:00Z',
});

describe('heldMessages', () => {
    it('keeps only chips that are still held', () => {
        const events = [
            ev('1', 'queued', 'later', '{"state":"held"}'),
            ev('2', 'queued', 'gone', '{"state":"removed"}'),
            ev('3', 'queued', 'sent', '{"state":"sent"}'),
            ev('4', 'user', 'plain', '{}'),
            ev('5', 'queued', 'broken', 'not json'),
        ];
        expect(heldMessages(events).map((e) => e.id)).toEqual(['1']);
        expect(heldMessages(null)).toEqual([]);
    });
});

describe('HeldMessages', () => {
    it('shows nothing without held messages', () => {
        const { container } = render(<HeldMessages held={[]} onEdit={vi.fn()} onRemove={vi.fn()} />);
        expect(container).toBeEmptyDOMElement();
    });

    it('offers edit and remove for each held message', () => {
        const held = [ev('1', 'queued', 'run the tests after this'), ev('2', 'queued', '')];
        const onEdit = vi.fn();
        const onRemove = vi.fn();
        render(<HeldMessages held={held} onEdit={onEdit} onRemove={onRemove} />);
        expect(screen.getAllByRole('listitem')).toHaveLength(2);
        expect(screen.getByText('run the tests after this')).toBeInTheDocument();
        fireEvent.click(screen.getAllByLabelText('Edit')[0]);
        fireEvent.click(screen.getAllByLabelText('Remove')[1]);
        expect(onEdit).toHaveBeenCalledWith(held[0]);
        expect(onRemove).toHaveBeenCalledWith(held[1]);
    });
});
