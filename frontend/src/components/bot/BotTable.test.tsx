import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import BotTable from './BotTable';
import type { BotSettings } from '@/types/bot';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (_: string, o?: { defaultValue?: string }) => o?.defaultValue ?? _ }) }));

describe('BotTable capabilities', () => {
    it('opens the selected bot and capability from desktop and mobile rows', () => {
        const onCapabilityClick = vi.fn();
        const bot = { uuid: 'b1', name: 'Bot', platform: 'telegram', enabled: true } as BotSettings;
        render(<BotTable bots={[bot]} onEdit={vi.fn()} onDelete={vi.fn()} onBotToggle={vi.fn()} onRestart={vi.fn()} onCapabilityClick={onCapabilityClick} />);
        fireEvent.click(screen.getAllByText('Remote')[0]);
        fireEvent.click(screen.getAllByText('Notify')[1]);
        expect(onCapabilityClick).toHaveBeenNthCalledWith(1, bot, 'remote_control');
        expect(onCapabilityClick).toHaveBeenNthCalledWith(2, bot, 'notify');
    });
});
