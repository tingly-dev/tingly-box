import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
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
        expect(await screen.findByText('status_message=root failure')).toBeInTheDocument();
        expect(screen.getByText('route=fallback')).toBeInTheDocument();
        expect(screen.getByText(/4 spans were dropped/)).toBeInTheDocument();
    });

    it.each([['responses gpt-5', 'responses'], ['chat gpt-5', 'chat']])('shows the %s root stage under its own operation', async (name, operation) => {
        render(<RequestJourney events={[]} traceId="trace" getTrace={vi.fn().mockResolvedValue({ trace_id: 'trace', spans: [
            { trace_id: 'trace', span_id: 'root', name, start_time: '2026-10-21T00:00:00Z', end_time: '2026-10-21T00:00:01Z', status_code: 'Ok', attributes: { 'gen_ai.operation.name': operation } },
        ] })} />);
        expect(await screen.findByText(operation)).toBeInTheDocument();
        expect(screen.getByText(`name=${name}`)).toBeInTheDocument();
        expect(screen.getByText(`gen_ai.operation.name=${operation}`)).toBeInTheDocument();
    });

    it('distinguishes a trace API failure from eviction', async () => {
        render(<RequestJourney events={[]} traceId="trace" getTrace={vi.fn().mockRejectedValue(new Error('HTTP 503'))} />);
        expect(await screen.findByText(/Failed to load stage timings: HTTP 503/)).toBeInTheDocument();
        expect(screen.queryByText(/no longer in the in-memory buffer/)).not.toBeInTheDocument();
    });

    const span = (id: string, name: string) => ({ trace_id: 'trace', span_id: id, name, start_time: '2026-10-21T00:00:00Z', end_time: '2026-10-21T00:00:01Z', status_code: 'Ok', attributes: { [`attr_${id}`]: 'v' } });
    const logEvent = { time: '2026-10-21T00:00:02Z', source: 'http', level: 'info', message: 'a log line', fields: { path: '/p' } };

    it('opens stage rows by default and leaves log rows folded', async () => {
        render(<RequestJourney events={[logEvent]} traceId="trace" getTrace={vi.fn().mockResolvedValue({ trace_id: 'trace', spans: [span('s1', 'routing')] })} />);
        expect(await screen.findByText('attr_s1=v')).toBeInTheDocument();
        expect(screen.queryByText('path=/p')).not.toBeInTheDocument();
        const [stageToggle, logToggle] = screen.getAllByRole('button');
        expect(stageToggle).toHaveAttribute('aria-expanded', 'true');
        expect(logToggle).toHaveAttribute('aria-expanded', 'false');
    });

    it('lets the reader fold an open stage', async () => {
        render(<RequestJourney events={[]} traceId="trace" getTrace={vi.fn().mockResolvedValue({ trace_id: 'trace', spans: [span('s1', 'routing')] })} />);
        fireEvent.click(await screen.findByText('routing'));
        await waitFor(() => expect(screen.queryByText('attr_s1=v')).not.toBeInTheDocument());
    });

    it('keeps stage rows and the reader\'s choices while a refresh re-fetches the trace', async () => {
        let resolveSecond!: (value: unknown) => void;
        const getTrace = vi.fn()
            .mockResolvedValueOnce({ trace_id: 'trace', spans: [span('s1', 'routing'), span('s2', 'upstream')] })
            .mockReturnValueOnce(new Promise((resolve) => { resolveSecond = resolve; }));
        const { rerender } = render(<RequestJourney events={[logEvent]} traceId="trace" getTrace={getTrace} />);
        fireEvent.click(await screen.findByText('routing'));
        fireEvent.click(screen.getByText('a log line'));
        await waitFor(() => expect(screen.queryByText('attr_s1=v')).not.toBeInTheDocument());
        expect(screen.getByText('path=/p')).toBeInTheDocument();

        rerender(<RequestJourney events={[logEvent, { ...logEvent, time: '2026-10-21T00:00:03Z', message: 'newer line' }]} traceId="trace" getTrace={getTrace} />);
        // The new trace is still in flight: the previous stages must not disappear.
        expect(screen.getByText('routing')).toBeInTheDocument();
        expect(screen.getByText('upstream')).toBeInTheDocument();
        expect(screen.getByText('attr_s2=v')).toBeInTheDocument();
        expect(screen.queryByText('attr_s1=v')).not.toBeInTheDocument();
        expect(screen.getByText('path=/p')).toBeInTheDocument();
        await act(async () => resolveSecond({ trace_id: 'trace', spans: [span('s1', 'routing'), span('s2', 'upstream')] }));
        expect(screen.queryByText('attr_s1=v')).not.toBeInTheDocument();
        expect(screen.getByText('attr_s2=v')).toBeInTheDocument();
    });

    it('shows the root span as its operation with the model as detail', async () => {
        render(<RequestJourney events={[]} traceId="trace" getTrace={vi.fn().mockResolvedValue({ trace_id: 'trace', spans: [
            { ...span('root', 'chat gpt-5.6-sol'), attributes: { 'gen_ai.operation.name': 'chat', 'gen_ai.request.model': 'gpt-5.6-sol', 'gen_ai.response.model': 'claude-sonnet-5', 'gen_ai.provider.name': 'Anthropic' } },
        ] })} />);
        expect(await screen.findByText('chat')).toBeInTheDocument();
        expect(screen.getByText('gpt-5.6-sol → claude-sonnet-5')).toBeInTheDocument();
        expect(screen.getByText('name=chat gpt-5.6-sol')).toBeInTheDocument();
        // The model route is never ellipsized: it wraps (and keeps a tooltip).
        expect(screen.getByText('gpt-5.6-sol → claude-sonnet-5')).toHaveAttribute('title', 'gpt-5.6-sol → claude-sonnet-5');
        expect(screen.getByText('gpt-5.6-sol → claude-sonnet-5')).toHaveStyle({ whiteSpace: 'normal' });
    });

    it('reads who-to-whom from the protocol attributes, and falls back to the bare model route without them', async () => {
        const root = { ...span('root', 'chat gpt-5.6-sol'), attributes: { 'gen_ai.operation.name': 'chat', 'gen_ai.request.model': 'gpt-5.6-sol', 'gen_ai.response.model': 'claude-sonnet-5', 'gen_ai.provider.name': 'Anthropic', 'tingly.protocol.inbound': 'openai.chat', 'tingly.protocol.upstream': 'anthropic.messages' } };
        const { unmount } = render(<RequestJourney events={[]} traceId="trace" getTrace={vi.fn().mockResolvedValue({ trace_id: 'trace', spans: [root] })} />);
        expect(await screen.findByText('request')).toBeInTheDocument();
        expect(screen.getByText('chat')).toBeInTheDocument();
        expect(screen.getByText('openai.chat · gpt-5.6-sol → anthropic.messages · claude-sonnet-5')).toBeInTheDocument();
        // The provider name is not in the row; it is in the opened details.
        expect(screen.queryByText(/Anthropic ·/)).not.toBeInTheDocument();
        expect(screen.getByText('gen_ai.provider.name=Anthropic')).toBeInTheDocument();
        unmount();

        const failed = { ...root, attributes: { 'gen_ai.operation.name': 'chat', 'gen_ai.request.model': 'gpt-5.6-sol', 'gen_ai.provider.name': 'OpenAI', 'tingly.protocol.inbound': 'openai.chat', 'tingly.protocol.upstream': 'openai.chat' } };
        render(<RequestJourney events={[]} traceId="trace2" getTrace={vi.fn().mockResolvedValue({ trace_id: 'trace2', spans: [failed] })} />);
        expect(await screen.findByText('openai.chat · gpt-5.6-sol → openai.chat')).toBeInTheDocument();
    });
});
