import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import AILogViewer from './AILogViewer';

const autoRefresh = vi.hoisted(() => ({ tick: undefined as undefined | (() => void) }));
vi.mock('@/hooks/useAutoRefresh', () => ({ useAutoRefresh: (_enabled: boolean, refresh: () => void) => { autoRefresh.tick = refresh; } }));
vi.mock('@/components/RequestJourney', () => ({ default: ({ events }: { events: { message: string }[] }) => <div>{events.map(e => e.message).join(' | ')}</div> }));

const request = { request_id: 'r1', time: '2026-10-21T00:00:00Z', request_model: 'test-model', has_error: false, event_count: 1 };

describe('AILogViewer detail refresh', () => {
    it('does not let a delayed detail response overwrite a newer snapshot', async () => {
        let resolveOlder!: (value: unknown) => void;
        const older = new Promise(resolve => { resolveOlder = resolve; });
        const getRequestDetail = vi.fn().mockReturnValueOnce(older)
            .mockResolvedValue({ ...request, events: [{ message: 'newest detail' }] });
        render(<AILogViewer getRequests={vi.fn().mockResolvedValue({ total: 1, requests: [request] })} getRequestDetail={getRequestDetail} getTrace={vi.fn()} />);
        fireEvent.click(await screen.findByText('test-model'));
        await waitFor(() => expect(getRequestDetail).toHaveBeenCalledTimes(1));
        fireEvent.click(screen.getByRole('button', { name: 'Refresh requests' }));
        expect(await screen.findByText('newest detail')).toBeInTheDocument();
        await act(async () => resolveOlder({ ...request, events: [{ message: 'stale detail' }] }));
        expect(screen.queryByText('stale detail')).not.toBeInTheDocument();
        expect(screen.getByText('newest detail')).toBeInTheDocument();
    });

    it('discards a pending detail response after switching requests', async () => {
        let resolveOlder!: (value: unknown) => void;
        const older = new Promise(resolve => { resolveOlder = resolve; });
        const second = { ...request, request_id: 'r2', request_model: 'second-model' };
        const getRequestDetail = vi.fn().mockReturnValueOnce(older)
            .mockResolvedValue({ ...second, events: [{ message: 'second request detail' }] });
        render(<AILogViewer getRequests={vi.fn().mockResolvedValue({ total: 2, requests: [request, second] })} getRequestDetail={getRequestDetail} getTrace={vi.fn()} />);
        fireEvent.click(await screen.findByText('test-model'));
        await waitFor(() => expect(getRequestDetail).toHaveBeenCalledTimes(1));
        fireEvent.click(screen.getByText('second-model'));
        expect(await screen.findByText('second request detail')).toBeInTheDocument();
        await act(async () => resolveOlder({ ...request, events: [{ message: 'first request detail' }] }));
        expect(screen.queryByText('first request detail')).not.toBeInTheDocument();
        expect(screen.getByText('second request detail')).toBeInTheDocument();
    });

    it('refreshes an open request instead of retaining a cached partial journey', async () => {
        const getRequestDetail = vi.fn()
            .mockResolvedValueOnce({ ...request, events: [{ message: 'started' }] })
            .mockResolvedValue({ ...request, events: [{ message: 'completed with diagnostic details' }] });
        render(<AILogViewer getRequests={vi.fn().mockResolvedValue({ total: 1, requests: [request] })} getRequestDetail={getRequestDetail} getTrace={vi.fn()} />);
        fireEvent.click(await screen.findByText('test-model'));
        expect(await screen.findByText('started')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Refresh requests' }));
        expect(await screen.findByText('completed with diagnostic details')).toBeInTheDocument();
        expect(getRequestDetail).toHaveBeenCalledTimes(2);
    });

    it('reports unavailable details and allows retry', async () => {
        const getRequestDetail = vi.fn().mockResolvedValue(null);
        render(<AILogViewer getRequests={vi.fn().mockResolvedValue({ total: 1, requests: [request] })} getRequestDetail={getRequestDetail} getTrace={vi.fn()} />);
        fireEvent.click(await screen.findByText('test-model'));
        expect(await screen.findByText(/Request details are no longer available/)).toBeInTheDocument();
        expect(screen.queryByText('Loading journey...')).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Refresh requests' }));
        await waitFor(() => expect(getRequestDetail).toHaveBeenCalledTimes(2));
    });

    describe('live updates while reading', () => {
        const newer = { ...request, request_id: 'r0', time: '2026-10-21T00:00:09Z', request_model: 'newer-model' };
        const poll = () => act(async () => { autoRefresh.tick?.(); });
        const rowOrder = () => screen.getAllByText(/-model$/).map(el => el.textContent);

        it('holds new requests behind a pill while a request is open, and applies them on click', async () => {
            const getRequests = vi.fn().mockResolvedValue({ total: 1, requests: [request] });
            render(<AILogViewer getRequests={getRequests} getRequestDetail={vi.fn().mockResolvedValue({ ...request, events: [{ message: 'detail' }] })} getTrace={vi.fn()} />);
            fireEvent.click(await screen.findByText('test-model'));
            expect(await screen.findByText('detail')).toBeInTheDocument();

            getRequests.mockResolvedValue({ total: 2, requests: [newer, request] });
            await poll();
            expect(rowOrder()).toEqual(['test-model']);
            const pill = await screen.findByText(/new request/);
            expect(screen.getByText('detail')).toBeInTheDocument();

            fireEvent.click(pill);
            await waitFor(() => expect(rowOrder()).toEqual(['newer-model', 'test-model']));
            expect(screen.getByText('detail')).toBeInTheDocument();
        });

        it('still updates a shown row in place and does not refetch the open detail for an unchanged row', async () => {
            const getRequests = vi.fn().mockResolvedValue({ total: 1, requests: [request] });
            const getRequestDetail = vi.fn().mockResolvedValue({ ...request, events: [{ message: 'detail' }] });
            render(<AILogViewer getRequests={getRequests} getRequestDetail={getRequestDetail} getTrace={vi.fn()} />);
            fireEvent.click(await screen.findByText('test-model'));
            await screen.findByText('detail');
            await poll();
            expect(getRequestDetail).toHaveBeenCalledTimes(1);

            getRequests.mockResolvedValue({ total: 1, requests: [{ ...request, latency_ms: 1500 }] });
            await poll();
            expect(await screen.findByText('1.5s')).toBeInTheDocument();
            await waitFor(() => expect(getRequestDetail).toHaveBeenCalledTimes(2));
            expect(screen.getByText('detail')).toBeInTheDocument();
        });

        it('inserts new requests straight away when nothing is open and the list is at the top', async () => {
            const getRequests = vi.fn().mockResolvedValue({ total: 1, requests: [request] });
            render(<AILogViewer getRequests={getRequests} getRequestDetail={vi.fn()} getTrace={vi.fn()} />);
            await screen.findByText('test-model');
            getRequests.mockResolvedValue({ total: 2, requests: [newer, request] });
            await poll();
            await waitFor(() => expect(rowOrder()).toEqual(['newer-model', 'test-model']));
            expect(screen.queryByText(/new request/)).not.toBeInTheDocument();
        });

        it('holds new requests while scrolled away from the top and releases them on returning', async () => {
            const getRequests = vi.fn().mockResolvedValue({ total: 1, requests: [request] });
            const { container } = render(<AILogViewer getRequests={getRequests} getRequestDetail={vi.fn()} getTrace={vi.fn()} />);
            await screen.findByText('test-model');
            const scroller = container.querySelector('.MuiTableContainer-root')!.parentElement!;
            scroller.scrollTop = 300;
            getRequests.mockResolvedValue({ total: 2, requests: [newer, request] });
            await poll();
            expect(rowOrder()).toEqual(['test-model']);
            expect(await screen.findByText(/new request/)).toBeInTheDocument();

            scroller.scrollTop = 0;
            fireEvent.scroll(scroller);
            await waitFor(() => expect(rowOrder()).toEqual(['newer-model', 'test-model']));
        });
    });
});
