import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { TemplatePageActions } from './TemplatePageActions';
import { TitleIconButtons } from './TitleIconButtons';

// No i18n instance in tests: t() returns the key, which is what we match on.
const base = {
    collapsible: true,
    allExpanded: true,
    onToggleExpandAll: vi.fn(),
    onAddApiKeyClick: vi.fn(),
    allowAddRule: true,
    onCreateRule: vi.fn(),
};

describe('TemplatePageActions', () => {
    it('shows the everyday actions as buttons, with no "⋯" menu', () => {
        const onViewLogs = vi.fn();
        render(<TemplatePageActions {...base} onProbeAll={vi.fn()} onViewLogs={onViewLogs} />);
        expect(screen.getByRole('button', { name: /probe.testAll/ })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /templateActions.connectAI/ })).toBeInTheDocument();
        expect(screen.queryByRole('button', { name: 'templateActions.more' })).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: 'templateActions.troubleshoot' }));
        expect(onViewLogs).toHaveBeenCalledOnce();
    });
});

describe('TitleIconButtons', () => {
    it('keeps fold and the guide as icon buttons beside the title, with no sort toggle', () => {
        const onShowGuide = vi.fn();
        const onToggleExpandAll = vi.fn();
        render(
            <TitleIconButtons
                collapsible
                allExpanded
                onToggleExpandAll={onToggleExpandAll}
                onShowGuide={onShowGuide}
            />,
        );
        expect(screen.queryByRole('button', { name: /sort/i })).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: 'templateActions.collapseAllRules' }));
        expect(onToggleExpandAll).toHaveBeenCalledOnce();
        fireEvent.click(screen.getByRole('button', { name: /howRoutingWorks|How routing works/ }));
        expect(onShowGuide).toHaveBeenCalledOnce();
    });
});
