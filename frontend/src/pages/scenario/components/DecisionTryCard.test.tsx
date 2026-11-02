import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Rule } from '@/components/RoutingGraphTypes';
import { runProbe } from '@/components/probe/runProbe';
import DecisionTryCard from './DecisionTryCard';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => ({
    'decisionTry.exampleInput': 'Invoice problem', 'decisionTry.exampleQuestion': 'Which team?', 'decisionTry.exampleOptions': 'billing\ntechnical',
    'decisionTry.examplePredicate': 'Is it urgent?', 'decisionTry.exampleScore': 'How urgent?', 'decisionTry.exampleLevels': 'low\nmedium\nhigh',
}[key] ?? key) }) }));
vi.mock('@/components/probe/runProbe', () => ({ runProbe: vi.fn(), formatLatency: (ms: number) => `${ms}ms` }));

const rule: Rule = { uuid: 'rule-1', scenario: 'decisions', request_model: 'decision-model', active: true };

describe('DecisionTryCard', () => {
    beforeEach(() => vi.resetAllMocks());

    it('offers a collapsible empty state without sending a request', () => {
        render(<DecisionTryCard rules={[]} />);
        expect(screen.getByText('decisionTry.noRules')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: /decisionTry.title/ }));
        expect(screen.getByRole('button', { name: /decisionTry.title/ })).toHaveAttribute('aria-expanded', 'false');
        expect(runProbe).not.toHaveBeenCalled();
    });

    it('runs the selected rule through Decisions and displays its answer', async () => {
        vi.mocked(runProbe).mockResolvedValue({ success: true, data: { latency_ms: 42, selected_provider: 'Provider', selected_model: 'upstream', decision_response: { answers: { result: { type: 'choice', choice: 'billing', probabilities: { billing: 0.9, technical: 0.1 } } } } } });
        render(<DecisionTryCard rules={[rule]} />);
        fireEvent.click(screen.getByRole('button', { name: 'decisionTry.run' }));
        await waitFor(() => expect(runProbe).toHaveBeenCalledWith({
            target_type: 'rule', scenario: 'decisions', rule_uuid: 'rule-1', operation: 'decisions',
            decision_request: { state: 'Invoice problem', questions: { result: { type: 'choice', instructions: 'Which team?', criteria: { billing: '', technical: '' } } } },
        }));
        expect(await screen.findByText('90.0%')).toBeInTheDocument();
        expect(screen.getByText('Provider · upstream · 42ms')).toBeInTheDocument();
    });

    it('requires a concrete model for a wildcard rule', async () => {
        vi.mocked(runProbe).mockResolvedValue({ success: false, error: { message: 'failed', type: 'probe_error' } });
        render(<DecisionTryCard rules={[{ ...rule, request_model: '*' }]} />);
        const run = screen.getByRole('button', { name: 'decisionTry.run' });
        expect(run).toBeDisabled();
        fireEvent.change(screen.getByLabelText('decisionTry.model'), { target: { value: 'actual-model' } });
        fireEvent.click(run);
        await waitFor(() => expect(runProbe).toHaveBeenCalledWith(expect.objectContaining({ model: 'actual-model' })));
        expect(await screen.findByText('failed')).toBeInTheDocument();
    });

    it('builds a score question with ordered levels', async () => {
        vi.mocked(runProbe).mockResolvedValue({ success: true, data: { latency_ms: 4, decision_response: { answers: { result: { type: 'score', score: 1.2, probabilities: { '0': 0.1, '1': 0.6, '2': 0.3 }, legend: { '0': 'low', '1': 'medium', '2': 'high' } } } } } });
        render(<DecisionTryCard rules={[rule]} />);
        fireEvent.mouseDown(screen.getByRole('combobox', { name: 'decisionTry.type' }));
        fireEvent.click(screen.getByRole('option', { name: 'decisionTry.score' }));
        fireEvent.click(screen.getByRole('button', { name: 'decisionTry.run' }));
        await waitFor(() => expect(runProbe).toHaveBeenCalledWith(expect.objectContaining({
            decision_request: { state: 'Invoice problem', questions: { result: { type: 'score', instructions: 'How urgent?', criteria: ['low', 'medium', 'high'] } } },
        })));
        expect(await screen.findByText('1.20')).toBeInTheDocument();
        expect(screen.getByText('60.0%')).toBeInTheDocument();
    });

    it('rejects duplicate choice values before sending', () => {
        render(<DecisionTryCard rules={[rule]} />);
        fireEvent.change(screen.getByLabelText('decisionTry.options'), { target: { value: 'same\nsame' } });
        expect(screen.getByRole('button', { name: 'decisionTry.run' })).toBeDisabled();
        expect(runProbe).not.toHaveBeenCalled();
    });

    it('builds a yes/no question and shows the returned probability', async () => {
        vi.mocked(runProbe).mockResolvedValue({ success: true, data: { latency_ms: 4, decision_response: { answers: { result: { type: 'noul', noul: 0.75 } } } } });
        render(<DecisionTryCard rules={[rule]} />);
        fireEvent.mouseDown(screen.getByRole('combobox', { name: 'decisionTry.type' }));
        fireEvent.click(screen.getByRole('option', { name: 'decisionTry.predicate' }));
        fireEvent.click(screen.getByRole('button', { name: 'decisionTry.run' }));
        await waitFor(() => expect(runProbe).toHaveBeenCalledWith(expect.objectContaining({
            decision_request: { state: 'Invoice problem', questions: { result: { type: 'noul', instructions: 'Is it urgent?' } } },
        })));
        expect(await screen.findByText('75.0%')).toBeInTheDocument();
    });
});
