import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { EntityLabel } from './EntityLabel';

describe('EntityLabel', () => {
    it('renders platform · alias · id for a direct chat and omits the group segment', () => {
        render(<EntityLabel platform="Telegram" alias="Alice" id="123456789" />);
        const el = screen.getByTestId('entity-label');
        expect(el.textContent).toBe('Telegram·Alice·123456789');
        expect(el.getAttribute('aria-label')).toBe('Telegram · Alice · 123456789');
    });

    it('puts the group segment after the alias', () => {
        render(<EntityLabel platform="Feishu" group groupLabel="群" alias="Dev team" id="oc_1" />);
        expect(screen.getByTestId('entity-label').textContent).toBe('Feishu·Dev team·群·oc_1');
    });

    it('drops the alias segment and its separator when there is none, and never renders blank', () => {
        render(<EntityLabel platform="Telegram" alias="  " id="42" />);
        expect(screen.getByTestId('entity-label').textContent).toBe('Telegram·42');
    });

    it('shortens a long id but keeps the full id in the accessible label and the copy value', async () => {
        const id = 'oc_a1b2c3d4e5f6a7b8c9d0e1f2a3b4d4e5';
        const onCopied = vi.fn();
        const writeText = vi.fn().mockResolvedValue(undefined);
        Object.assign(navigator, { clipboard: { writeText } });
        Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true });
        render(<EntityLabel platform="Feishu" group groupLabel="群" alias="Dev" id={id} copy onCopied={onCopied} />);
        const el = screen.getByTestId('entity-label');
        expect(el.textContent).toContain('oc_a1b…d4e5');
        expect(el.textContent).not.toContain(id);
        expect(el.getAttribute('aria-label')).toContain(id);
        fireEvent.click(screen.getByRole('button', { name: 'Copy ID' }));
        await waitFor(() => expect(writeText).toHaveBeenCalledWith(id));
    });

    it('marks a deleted entity and keeps its id', () => {
        render(<EntityLabel platform="Telegram" id="55512345" resolved={false} deletedLabel="(deleted)" />);
        expect(screen.getByTestId('entity-label').textContent).toBe('Telegram·55512345(deleted)');
    });

    it('can hide the platform text for rows under a bot header while keeping it accessible', () => {
        render(<EntityLabel platform="Telegram" platformIcon={<i data-testid="icon" />} platformAs="icon" alias="Alice" id="1" />);
        const el = screen.getByTestId('entity-label');
        expect(el.textContent).toBe('Alice·1');
        expect(screen.getByTestId('icon')).toBeTruthy();
        expect(el.getAttribute('aria-label')).toBe('Telegram · Alice · 1');
    });

    it('compact leaves the id out when there is an alias but keeps the full label reachable', () => {
        render(<EntityLabel platform="Feishu" group groupLabel="群" alias="Dev team" id="oc_a1b2c3d4e5f6a7b8c9d0e1f2a3b4d4e5" compact />);
        const el = screen.getByTestId('entity-label');
        expect(el.textContent).toBe('Feishu·Dev team·群');
        expect(el.getAttribute('title')).toBe('Feishu · Dev team · 群 · oc_a1b2c3d4e5f6a7b8c9d0e1f2a3b4d4e5');
        expect(el.getAttribute('aria-label')).toBe(el.getAttribute('title'));
    });

    it('compact still shows the id when there is no alias', () => {
        render(<EntityLabel platform="Telegram" id="123456789" compact />);
        expect(screen.getByTestId('entity-label').textContent).toBe('Telegram·123456789');
    });

    it('shortens bot ids by their distinguishing tail', () => {
        render(<EntityLabel platform="Telegram" kind="bot" alias="@ops_bot" id="1730600000123456789-imbot" />);
        expect(screen.getByTestId('entity-label').textContent).toBe('Telegram·@ops_bot·…456789');
    });
});
