import {act, cleanup, fireEvent, render, screen} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {useTranscriptScroll} from './useTranscriptScroll';

let resize: ResizeObserverCallback;
const disconnect = vi.fn();
beforeEach(() => {
    vi.stubGlobal('ResizeObserver', class {
        constructor(callback: ResizeObserverCallback) {resize = callback;}
        observe() {}
        disconnect = disconnect;
    });
});
afterEach(() => {cleanup(); vi.unstubAllGlobals(); vi.clearAllMocks();});

function Reader({revision}: {revision: string}) {
    const {viewport: viewportRef, content: contentRef, onScroll, latest, away, unread} = useTranscriptScroll(revision);
    return <><div ref={viewportRef} data-testid="viewport" onScroll={onScroll}><div ref={contentRef}/></div>
        <button onClick={latest}>Latest</button><span>{away ? 'reading earlier' : 'following'}</span><span>{unread ? 'new output' : 'seen'}</span></>;
}
const resized = () => act(() => resize([], {} as ResizeObserver));

describe('Desk transcript reading', () => {
    it('follows delayed output layout while at the bottom and preserves reading position through polls', () => {
        const view = render(<Reader revision="first"/>);
        const viewport = screen.getByTestId('viewport');
        Object.defineProperties(viewport, {scrollHeight: {value: 1000, configurable: true}, clientHeight: {value: 400}});
        resized();
        expect(viewport.scrollTop).toBe(1000);
        viewport.scrollTop = 100;
        fireEvent.scroll(viewport);
        view.rerender(<Reader revision="first"/>);
        expect(screen.getByText('seen')).toBeInTheDocument();
        view.rerender(<Reader revision="second"/>);
        Object.defineProperty(viewport, 'scrollHeight', {value: 1400});
        resized();
        expect(viewport.scrollTop).toBe(100);
        expect(screen.getByText('new output')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', {name: 'Latest'}));
        expect(viewport.scrollTop).toBe(1400);
        expect(screen.getByText('following')).toBeInTheDocument();
        expect(screen.getByText('seen')).toBeInTheDocument();
        view.unmount();
        expect(disconnect).toHaveBeenCalledOnce();
    });
});
