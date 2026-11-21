import { fireEvent, render, screen, within } from '@testing-library/react';
import type { ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { PhoneShellContext } from '@tingly/ui/hooks/usePhoneShell';
import UnifiedRoutingGraph from '@/components/UnifiedRoutingGraph';
import type { ConfigRecord } from '@/components/RoutingGraphTypes';
import type { Provider } from '@/types/provider';
import { PhoneRuleTargets } from './PhoneRuleTargets';

// t() returns the key, which is what we match on.
vi.mock('react-i18next', async (importOriginal) => ({
    ...await importOriginal<typeof import('react-i18next')>(),
    useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }),
}));

const providers = [
    { uuid: 'p-openai', name: 'OpenAI', api_style: 'openai', enabled: true },
    { uuid: 'p-ds', name: 'DeepSeek', api_style: 'openai', enabled: true },
] as unknown as Provider[];

const record: ConfigRecord = {
    uuid: 'rule-1',
    requestModel: 'claude-sonnet-5',
    responseModel: '',
    active: true,
    providers: [
        { uuid: 's-sol', provider: 'p-openai', model: 'gpt-5.6-sol', tier: 0 },
        { uuid: 's-ds', provider: 'p-ds', model: 'deepseek-v4-flash', tier: 1 },
    ],
    smartEnabled: true,
    smartRouting: [{
        uuid: 'c-1',
        description: '',
        ops: [{ uuid: 'op', position: 'token', operation: 'ge', value: '60000' }],
        services: [{ uuid: 's-pro', provider: 'p-ds', model: 'deepseek-v4-pro' }],
    }],
};

const phone = (ui: ReactNode) => render(<PhoneShellContext.Provider value>{ui}</PhoneShellContext.Provider>);

describe('rule card in the phone shell', () => {
    it('lists the rule’s conditions and tiers instead of drawing the graph', () => {
        phone(<UnifiedRoutingGraph record={record} providers={providers} />);

        expect(screen.getByText('IF 1')).toBeInTheDocument();
        expect(screen.getByText('token ge 60000')).toBeInTheDocument();
        expect(screen.getByText('rule.ruleConditions.othersTitle · T0')).toBeInTheDocument();
        expect(screen.getByText('T1')).toBeInTheDocument();
        expect(screen.getByText('deepseek-v4-pro')).toBeInTheDocument();
        // Conditions are edited on the desktop, and the card says so.
        expect(screen.getByText('rule.phone.conditionsOnDesktop')).toBeInTheDocument();
        // No graph nodes: the add-condition node is desktop-only.
        expect(screen.queryByText('rule.ruleConditions.addCondition')).not.toBeInTheDocument();
    });

    it('keeps the graph on the desktop', () => {
        render(<UnifiedRoutingGraph record={record} providers={providers} />);
        expect(screen.queryByText('rule.phone.conditionsOnDesktop')).not.toBeInTheDocument();
        expect(screen.getByText('rule.ruleConditions.addCondition')).toBeInTheDocument();
    });
});

describe('PhoneRuleTargets', () => {
    const setup = () => {
        const handlers = {
            onServiceNodeClick: vi.fn(),
            onAddService: vi.fn(),
            onAddServiceToSmartRule: vi.fn(),
            onDeleteService: vi.fn(),
            onTierChange: vi.fn(),
        };
        render(<PhoneRuleTargets record={record} providers={providers} active saving={false} {...handlers} />);
        return handlers;
    };

    it('opens the model picker on the target you tap', () => {
        const h = setup();
        fireEvent.click(screen.getByText('gpt-5.6-sol'));
        expect(h.onServiceNodeClick).toHaveBeenCalledWith('s-sol');
    });

    it('adds a model to a tier, a condition, or a new fallback tier', () => {
        const h = setup();
        const adds = screen.getAllByRole('button', { name: 'rule.nodes.addModel' });
        // The condition's add, then T0's, then T1's.
        fireEvent.click(adds[0]);
        expect(h.onAddServiceToSmartRule).toHaveBeenCalledWith('c-1');
        fireEvent.click(adds[2]);
        expect(h.onAddService).toHaveBeenCalledWith(1);
        fireEvent.click(screen.getByRole('button', { name: 'rule.nodes.fallbackTier' }));
        expect(h.onAddService).toHaveBeenLastCalledWith(2);
    });

    it('moves a target between tiers and deletes it only after confirming', () => {
        const h = setup();
        const row = screen.getByText('deepseek-v4-flash').closest('.MuiBox-root')!.parentElement!.parentElement as HTMLElement;
        fireEvent.click(within(row).getByRole('button', { name: 'rule.phone.targetActions' }));
        fireEvent.click(screen.getByRole('menuitem', { name: 'common.moveUp' }));
        expect(h.onTierChange).toHaveBeenCalledWith('s-ds', 0);

        fireEvent.click(within(row).getByRole('button', { name: 'rule.phone.targetActions' }));
        fireEvent.click(screen.getByRole('menuitem', { name: 'rule.service.deleteService' }));
        expect(h.onDeleteService).not.toHaveBeenCalled();
        fireEvent.click(screen.getByRole('button', { name: 'common.delete' }));
        expect(h.onDeleteService).toHaveBeenCalledWith('s-ds');
    });
});
