import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ReactNode } from 'react';
import TTSPage from './TTSPage';

const { create, getClient } = vi.hoisted(() => ({ create: vi.fn(), getClient: vi.fn() }));
vi.mock('@/services/modelApi', () => ({ getOpenAIClient: getClient }));
vi.mock('./AudioApiPage', () => ({ default: ({ children }: { children: (models: string[]) => ReactNode }) => children(['speaker']) }));

describe('speech playground', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        getClient.mockResolvedValue({ audio: { speech: { create } } });
        URL.createObjectURL = vi.fn(() => 'blob:speech');
        URL.revokeObjectURL = vi.fn();
    });

    it('uses the TTS gateway, exposes playback/download and revokes the URL on unmount', async () => {
        create.mockResolvedValue({ blob: async () => new Blob(['audio'], { type: 'audio/mpeg' }) });
        const view = render(<TTSPage />);
        expect(screen.getByRole('button', { name: 'audio.generate' })).toBeDisabled();
        fireEvent.change(screen.getByLabelText('audio.input'), { target: { value: 'Hello' } });
        fireEvent.click(screen.getByRole('button', { name: 'audio.generate' }));
        const link = await screen.findByRole('link', { name: 'audio.download' });
        expect(getClient).toHaveBeenCalledWith('tts');
        expect(create).toHaveBeenCalledWith(expect.objectContaining({ model: 'speaker', input: 'Hello', voice: 'alloy', response_format: 'mp3' }), expect.objectContaining({ signal: expect.any(AbortSignal) }));
        expect(link).toHaveAttribute('download', 'speech.mp3');
        expect(screen.getByLabelText('audio.preview')).toHaveAttribute('src', 'blob:speech');
        view.unmount();
        expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:speech');
    });

    it('shows upstream errors and lets the user try again', async () => {
        create.mockRejectedValue(new Error('Unsupported voice'));
        render(<TTSPage />);
        fireEvent.change(screen.getByLabelText('audio.input'), { target: { value: 'Hello' } });
        fireEvent.click(screen.getByRole('button', { name: 'audio.generate' }));
        expect(await screen.findByRole('alert')).toHaveTextContent('Unsupported voice');
        expect(screen.queryByRole('link', { name: 'audio.download' })).toBeNull();
        expect(screen.getByRole('button', { name: 'audio.generate' })).toBeEnabled();
    });

    it('cancels the fetch and discards a result that arrives after cancellation', async () => {
        let finish!: (value: { blob: () => Promise<Blob> }) => void;
        create.mockImplementation(() => new Promise(resolve => { finish = resolve; }));
        render(<TTSPage />);
        fireEvent.change(screen.getByLabelText('audio.input'), { target: { value: 'Hello' } });
        fireEvent.click(screen.getByRole('button', { name: 'audio.generate' }));
        await waitFor(() => expect(create).toHaveBeenCalled());
        fireEvent.click(screen.getByRole('button', { name: 'audio.cancel' }));
        expect(create.mock.calls[0][1].signal.aborted).toBe(true);
        await act(async () => { finish({ blob: async () => new Blob(['late']) }); });
        expect(URL.createObjectURL).not.toHaveBeenCalled();
        expect(screen.getByRole('button', { name: 'audio.generate' })).toBeEnabled();
    });
});
