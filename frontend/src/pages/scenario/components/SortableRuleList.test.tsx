import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Rule } from '@/components/RoutingGraphTypes';
import { SortableRuleList } from './SortableRuleList';

// No i18n instance in tests: t() returns the key, which is what we match on.
const rule = (uuid: string): Rule => ({ uuid, request_model: `model-${uuid}` } as unknown as Rule);
const RULES = ['a', 'b', 'c'].map(rule);

// jsdom has no layout: give each rule a 100px-tall row so the keyboard
// sensor can find the neighbour in the arrow-key direction.
const rowTop = (el: Element) => {
    const row = el.hasAttribute('data-row') ? el : el.querySelector('[data-row]');
    return row ? Number(row.getAttribute('data-row')) * 100 : 0;
};
let rectSpy: ReturnType<typeof vi.spyOn>;
beforeEach(() => {
    rectSpy = vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (this: Element) {
        const top = rowTop(this);
        return { top, bottom: top + 90, left: 0, right: 300, width: 300, height: 90, x: 0, y: top, toJSON: () => ({}) } as DOMRect;
    });
});
afterEach(() => rectSpy.mockRestore());

// dnd-kit measures the drop targets on a timer after the lift.
const settle = () => act(async () => { await new Promise(resolve => setTimeout(resolve, 50)); });

const renderList = (props: Partial<React.ComponentProps<typeof SortableRuleList>> = {}) => {
    const onReorder = vi.fn();
    render(
        <SortableRuleList
            rules={RULES}
            enabled
            onReorder={onReorder}
            renderRule={(r, handle) => (
                <div data-row={RULES.findIndex(x => x.uuid === r.uuid)} data-testid={`rule-${r.uuid}`}>
                    {r.request_model}{handle}
                </div>
            )}
            {...props}
        />,
    );
    return { onReorder };
};

describe('SortableRuleList', () => {
    it('gives every rule a labelled drag handle', () => {
        renderList();
        expect(screen.getAllByRole('button', { name: 'templatePage.reorder.handle' })).toHaveLength(3);
    });

    it('renders plain rules with no handles when disabled (name-sorted view, slot pages)', () => {
        renderList({ enabled: false });
        expect(screen.queryByRole('button', { name: 'templatePage.reorder.handle' })).toBeNull();
        expect(screen.getByText('model-a')).toBeInTheDocument();
    });

    it('reorders with the keyboard: space lifts, arrow moves, space drops', async () => {
        const { onReorder } = renderList();
        const handle = screen.getAllByRole('button', { name: 'templatePage.reorder.handle' })[0];
        handle.focus();
        await act(async () => { fireEvent.keyDown(handle, { code: 'Space', key: ' ' }); });
        await settle();
        await act(async () => { fireEvent.keyDown(document, { code: 'ArrowDown', key: 'ArrowDown' }); });
        await act(async () => { fireEvent.keyDown(document, { code: 'Space', key: ' ' }); });
        expect(onReorder).toHaveBeenCalledWith(['b', 'a', 'c']);
    });

    it('announces the move for screen readers', async () => {
        renderList();
        const handle = screen.getAllByRole('button', { name: 'templatePage.reorder.handle' })[0];
        handle.focus();
        await act(async () => { fireEvent.keyDown(handle, { code: 'Space', key: ' ' }); });
        await settle();
        await act(async () => { fireEvent.keyDown(document, { code: 'ArrowDown', key: 'ArrowDown' }); });
        expect(document.body.textContent).toContain('templatePage.reorder.announce.over');
        await act(async () => { fireEvent.keyDown(document, { code: 'Space', key: ' ' }); });
        expect(document.body.textContent).toContain('templatePage.reorder.announce.end');
    });

    it('cancels with escape without reordering', async () => {
        const { onReorder } = renderList();
        const handle = screen.getAllByRole('button', { name: 'templatePage.reorder.handle' })[1];
        handle.focus();
        await act(async () => { fireEvent.keyDown(handle, { code: 'Space', key: ' ' }); });
        await settle();
        await act(async () => { fireEvent.keyDown(document, { code: 'ArrowDown', key: 'ArrowDown' }); });
        await act(async () => { fireEvent.keyDown(document, { code: 'Escape', key: 'Escape' }); });
        expect(onReorder).not.toHaveBeenCalled();
    });
});
