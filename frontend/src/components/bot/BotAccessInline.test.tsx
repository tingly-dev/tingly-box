import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import BotAccessDialog from './BotAccessDialog';
import type { BotSettings } from '@/types/bot';
import type { RemoteAccess } from './useRemoteAccess';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (_: string, options?: { defaultValue?: string }) => options?.defaultValue ?? _ }) }));
vi.mock('./PairingCodePanel', () => ({ default: () => null }));

describe('inline remote access', () => {
    it('shows chat and group controls without opening another dialog', () => {
        const access: RemoteAccess = { loaded: true, loading: false, error: '', chats: [], groups: [], controllers: [], controllingGroups: [], pendingChats: [], reload: vi.fn().mockResolvedValue(undefined) };
        render(<BotAccessDialog inline open scope="remote_control" bot={{ uuid: 'bot', platform: 'telegram' } as BotSettings} onClose={vi.fn()} remoteAccess={access} />);
        expect(screen.getByText('Direct Chats')).toBeInTheDocument();
        expect(screen.getByText('Groups')).toBeInTheDocument();
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });

    it('edits a chat alias in place', () => {
        const chat = { chat: { id: 'c1', bot_uuid: 'bot', platform: 'telegram', external_chat_id: '123', alias: 'Alice', peer_actor_id: 'a1', blocked: false }, permissions: [] };
        const access: RemoteAccess = { loaded: true, loading: false, error: '', chats: [chat], groups: [], controllers: [], controllingGroups: [], pendingChats: [chat], reload: vi.fn().mockResolvedValue(undefined) };
        render(<BotAccessDialog inline open scope="remote_control" bot={{ uuid: 'bot', platform: 'telegram' } as BotSettings} onClose={vi.fn()} remoteAccess={access} />);
        fireEvent.click(screen.getByRole('button', { name: 'Rename' }));
        expect(screen.getByLabelText('Alias')).toHaveValue('Alice');
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
});
