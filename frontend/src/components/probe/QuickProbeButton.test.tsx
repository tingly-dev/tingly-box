import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { runProbe } from './runProbe';
import { QuickProbeButton } from './QuickProbeButton';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('./runProbe', () => ({ runProbe: vi.fn(), formatLatency: (ms: number) => `${ms}ms` }));
vi.mock('./ProbeDialog', () => ({ ProbeDialog: () => null }));

describe('Decision quick probe', () => {
    it('uses the decision operation instead of chat axes', async () => {
        vi.mocked(runProbe).mockResolvedValue({ success: true, data: { latency_ms: 20, decision_response: { answers: { team: { type: 'choice', choice: 'billing' } } } } });
        render(<QuickProbeButton ruleUuid="r" ruleName="model" scenario="decisions" model="model" />);
        fireEvent.click(screen.getByRole('button', { name: 'ruleHeader.test' }));
        await waitFor(() => expect(runProbe).toHaveBeenCalledWith({ target_type: 'rule', scenario: 'decisions', rule_uuid: 'r', operation: 'decisions' }));
        expect(await screen.findByText('20ms')).toBeInTheDocument();
    });
    it('does not guess a model for wildcard rules', () => {
        render(<QuickProbeButton ruleUuid="r" ruleName="*" scenario="decisions" model="*" />);
        expect(screen.getByRole('button', { name: 'ruleHeader.test' })).toBeDisabled();
    });
});
