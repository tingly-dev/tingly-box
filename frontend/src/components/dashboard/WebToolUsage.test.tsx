import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import WebToolUsage from './WebToolUsage';
import type { AggregatedStat } from './ServiceStatsTable';
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

describe('web tool usage is separate from model metrics', () => {
    it('shows requests, failures and estimated spend without a token or model-rate column', () => {
        const stats: AggregatedStat[] = [{ key: 'deepseek-tools/search', model: 'deepseek-tools/search', provider_name: 'DeepSeek', request_count: 5, billable_requests: 3, request_cost_cny: .015, error_count: 1, total_input_tokens: 0, total_output_tokens: 0, avg_latency_ms: 400, error_rate: .2 }];
        render(<WebToolUsage stats={stats} />);
        expect(screen.getByRole('table')).toBeInTheDocument();
        expect(screen.getByText('search')).toBeInTheDocument();
        expect(screen.getByText('dashboard.webToolEstimateHint')).toBeInTheDocument();
        expect(screen.getByText('5')).toBeInTheDocument();
        expect(screen.getByText('3')).toBeInTheDocument();
        expect(screen.getAllByText(/CNY 0.015/)).toHaveLength(2);
        expect(screen.queryByText(/token/i)).not.toBeInTheDocument();
    });
    it('does not imply zero cost before there are tool records', () => {
        const { container } = render(<WebToolUsage stats={[]} />);
        expect(container).toBeEmptyDOMElement();
    });
});
