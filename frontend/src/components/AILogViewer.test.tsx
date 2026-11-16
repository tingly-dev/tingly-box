import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import AILogViewer from './AILogViewer';

vi.mock('@/hooks/useAutoRefresh', () => ({ useAutoRefresh: vi.fn() }));
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
});
