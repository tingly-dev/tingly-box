import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import GraphSettingsMenu from './GraphSettingsMenu';
import ModelRequestHeader from './ModelRequestHeader';

// No i18n instance in tests: t() returns the key, which is what we match on.
describe('rule header plugins', () => {
    it('shows the Plugins tag only when the rule has plugins', () => {
        const onPluginsClick = vi.fn();
        const { rerender } = render(
            <ModelRequestHeader modelName="m" pluginsCount={0} onPluginsClick={onPluginsClick} />,
        );
        expect(screen.queryByRole('button', { name: /ruleHeader.plugins/ })).toBeNull();

        rerender(<ModelRequestHeader modelName="m" pluginsCount={4} onPluginsClick={onPluginsClick} />);
        fireEvent.click(screen.getByRole('button', { name: 'ruleHeader.plugins 4' }));
        expect(onPluginsClick).toHaveBeenCalledOnce();
    });

    it('keeps the Endpoint chip (and its menu) even without an OpenAI-compatible upstream', () => {
        const onEndpointSelect = vi.fn();
        render(<ModelRequestHeader modelName="m" onEndpointSelect={onEndpointSelect} endpointApplicable={false} />);
        const chip = screen.getByRole('button', { name: 'Upstream endpoint: auto' });
        expect(chip).toBeEnabled();
        fireEvent.click(chip);
        fireEvent.click(screen.getByRole('menuitem', { name: /^Chat Always/ }));
        expect(onEndpointSelect).toHaveBeenCalledWith('chat');
    });

    it('adds / removes plugins from the "⋯" menu', () => {
        const onOpenPlugins = vi.fn();
        render(
            <GraphSettingsMenu
                allowDeleteRule
                active
                allowToggleRule
                saving={false}
                onDelete={vi.fn()}
                onToggleActive={vi.fn()}
                onOpenPlugins={onOpenPlugins}
                pluginsCount={2}
            />,
        );
        fireEvent.click(screen.getByRole('button', { name: 'ruleHeader.ruleActions' }));
        fireEvent.click(screen.getByRole('menuitem', { name: /ruleHeader.plugins/ }));
        expect(onOpenPlugins).toHaveBeenCalledOnce();
    });
});
