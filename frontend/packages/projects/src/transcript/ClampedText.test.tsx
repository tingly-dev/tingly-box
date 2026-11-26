import {cleanup, fireEvent, render, screen} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import ClampedText from './ClampedText';

vi.mock('react-i18next', () => ({useTranslation: () => ({t: (_key: string, opts?: {defaultValue?: string}) => opts?.defaultValue ?? _key})}));

// jsdom has no layout: give the clamp body the two numbers it measures.
let scrollHeight = 400;
let clientHeight = 100;
beforeEach(() => {
    vi.spyOn(HTMLElement.prototype, 'scrollHeight', 'get').mockImplementation(() => scrollHeight);
    vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockImplementation(() => clientHeight);
});
afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
});

describe('ClampedText', () => {
    it('leaves short text alone: no row, no clip', () => {
        scrollHeight = 50;
        clientHeight = 50;
        render(<ClampedText lines={10}>a short message</ClampedText>);
        expect(screen.queryByRole('button', {name: 'Show full text'})).toBeNull();
        expect(screen.getByText('a short message')).toBeInTheDocument();
    });

    it('clamps a long message and expands it in place', () => {
        scrollHeight = 400;
        clientHeight = 100;
        render(<ClampedText lines={10}>a long message</ClampedText>);
        const expand = screen.getByRole('button', {name: 'Show full text'});
        expect(expand).toHaveAttribute('aria-expanded', 'false');
        expect(expand).toHaveAttribute('aria-controls');
        fireEvent.click(expand);
        expect(screen.getByRole('button', {name: 'Show less'})).toHaveAttribute('aria-expanded', 'true');
        // The full text is always in the DOM; the row only changes the view.
        expect(screen.getByText('a long message')).toBeInTheDocument();
    });
});
