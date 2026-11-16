import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import RequestJourney from './RequestJourney';

describe('RequestJourney diagnostics', () => {
    it('keeps the same event open when an earlier event arrives on refresh', () => {
        const event = { time: '2026-10-21T00:00:01Z', source: 'http', level: 'error', message: 'request failed', fields: { error: 'original error detail' } };
        const getTrace = vi.fn();
        const { rerender } = render(<RequestJourney events={[event]} getTrace={getTrace} />);
        fireEvent.click(screen.getByText('request failed'));
        expect(screen.getByText('error=original error detail')).toBeInTheDocument();
        rerender(<RequestJourney events={[
            { time: '2026-10-21T00:00:00Z', source: 'model_request', level: 'info', message: 'late arriving stage', fields: { provider: 'test-provider' } },
            event,
        ]} getTrace={getTrace} />);
        expect(screen.getByText('error=original error detail')).toBeInTheDocument();
        expect(screen.queryByText('provider=test-provider')).not.toBeInTheDocument();
    });

    it('retains the HTTP error and full text alongside stage logs', () => {
        const message = 'HTTP request failed\nsecond line with diagnostic details';
        render(<RequestJourney getTrace={vi.fn()} events={[
            { time: '2026-10-21T00:00:00Z', source: 'model_request', level: 'info', stage: 'upstream', message: 'upstream started' },
            { time: '2026-10-21T00:00:01Z', source: 'http', level: 'error', message, fields: { error: 'provider rejected request', path: '/tingly/openai/v1/responses', status: 502 } },
        ]} />);
        fireEvent.click(screen.getByText(/HTTP request failed/));
        expect(screen.getByText('error=provider rejected request')).toBeInTheDocument();
        expect(screen.getByText('path=/tingly/openai/v1/responses')).toBeInTheDocument();
        expect(screen.getByText(/message=HTTP request failed/)).toHaveTextContent('second line with diagnostic details');
    });

    it('shows a root span and reports dropped spans', async () => {
        render(<RequestJourney events={[]} traceId="trace" getTrace={vi.fn().mockResolvedValue({ trace_id: 'trace', dropped_spans: 4, spans: [
            { trace_id: 'trace', span_id: 'root', name: 'chat model', start_time: '2026-10-21T00:00:00Z', end_time: '2026-10-21T00:00:01Z', status_code: 'Error', status_message: 'root failure', attributes: { route: 'fallback' } },
        ] })} />);
        fireEvent.click(await screen.findByText('chat model'));
        expect(screen.getByText('status_message=root failure')).toBeInTheDocument();
        expect(screen.getByText('route=fallback')).toBeInTheDocument();
        expect(screen.getByText(/4 spans were dropped/)).toBeInTheDocument();
    });

    it('distinguishes a trace API failure from eviction', async () => {
        render(<RequestJourney events={[]} traceId="trace" getTrace={vi.fn().mockRejectedValue(new Error('HTTP 503'))} />);
        expect(await screen.findByText(/Failed to load stage timings: HTTP 503/)).toBeInTheDocument();
        expect(screen.queryByText(/no longer in the in-memory buffer/)).not.toBeInTheDocument();
    });
});
