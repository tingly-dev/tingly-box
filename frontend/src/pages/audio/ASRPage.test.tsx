import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ASRPage from './ASRPage';

const { transcribe, translate, getClient } = vi.hoisted(() => ({ transcribe: vi.fn(), translate: vi.fn(), getClient: vi.fn() }));
vi.mock('@/services/modelApi', () => ({ getOpenAIClient: getClient }));
vi.mock('./AudioApiPage', () => ({ default: ({ children }: { children: (models: string[]) => ReactNode }) => children(['listener']) }));

const upload = () => fireEvent.change(screen.getByLabelText('audio.upload'), { target: { files: [new File(['audio'], 'speech.wav', { type: 'audio/wav' })] } });

describe('recognition playground', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        getClient.mockResolvedValue({ audio: { transcriptions: { create: transcribe }, translations: { create: translate } } });
        URL.createObjectURL = vi.fn(() => 'blob:transcript'); URL.revokeObjectURL = vi.fn();
    });

    it('uploads the selected file through ASR and downloads the transcript', async () => {
        transcribe.mockReturnValue({ asResponse: async () => new Response('{"text":"Hello"}', { headers: { 'Content-Type': 'application/json' } }) });
        const view = render(<ASRPage />);
        expect(screen.getByRole('button', { name: 'audio.recognize' })).toBeDisabled();
        upload(); fireEvent.click(screen.getByRole('button', { name: 'audio.recognize' }));
        const download = await screen.findByRole('link', { name: 'audio.downloadTranscript' });
        expect(getClient).toHaveBeenCalledWith('asr');
        expect(transcribe).toHaveBeenCalledWith(expect.objectContaining({ model: 'listener', file: expect.any(File), response_format: 'json' }), expect.objectContaining({ signal: expect.any(AbortSignal) }));
        expect(screen.getByLabelText('audio.transcript')).toHaveValue('{\n  "text": "Hello"\n}');
        expect(download).toHaveAttribute('download', 'transcript.json');
        view.unmount(); expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:transcript');
    });

    it('renders stream deltas and replaces them with the final text', async () => {
        transcribe.mockResolvedValue((async function* () {
            yield { type: 'transcript.text.delta', delta: 'Hel' };
            yield { type: 'transcript.text.delta', delta: 'lo' };
            yield { type: 'transcript.text.done', text: 'Hello world' };
        })());
        render(<ASRPage />); upload();
        fireEvent.click(screen.getByLabelText('audio.streaming'));
        fireEvent.click(screen.getByRole('button', { name: 'audio.recognize' }));
        await screen.findByRole('link', { name: 'audio.downloadTranscript' });
        expect(transcribe.mock.calls[0][0].stream).toBe(true);
        expect(screen.getByLabelText('audio.transcript')).toHaveValue('Hello world');
        expect(screen.getByRole('link', { name: 'audio.downloadTranscript' })).toHaveAttribute('download', 'transcript.txt');
    });

    it('routes translation separately and preserves subtitle text for download', async () => {
        const subtitle = '1\n00:00:00,000 --> 00:00:01,000\nHello\n';
        translate.mockReturnValue({ asResponse: async () => new Response(subtitle) });
        render(<ASRPage />); upload();
        fireEvent.mouseDown(screen.getByRole('combobox', { name: 'audio.task' }));
        fireEvent.click(screen.getByRole('option', { name: 'audio.translate' }));
        fireEvent.mouseDown(screen.getByRole('combobox', { name: 'audio.format' }));
        fireEvent.click(screen.getByRole('option', { name: 'srt' }));
        fireEvent.click(screen.getByRole('button', { name: 'audio.recognize' }));
        const link = await screen.findByRole('link', { name: 'audio.downloadTranscript' });
        expect(translate).toHaveBeenCalledWith(expect.objectContaining({ model: 'listener', response_format: 'srt' }), expect.anything());
        expect(transcribe).not.toHaveBeenCalled();
        expect(screen.getByLabelText('audio.transcript')).toHaveValue(subtitle);
        expect(link).toHaveAttribute('download', 'transcript.srt');
    });

    it('cancels requests and discards late transcripts', async () => {
        let finish!: (response: Response) => void;
        transcribe.mockReturnValue({ asResponse: () => new Promise(resolve => { finish = resolve; }) });
        render(<ASRPage />); upload(); fireEvent.click(screen.getByRole('button', { name: 'audio.recognize' }));
        await waitFor(() => expect(transcribe).toHaveBeenCalled());
        fireEvent.click(screen.getByRole('button', { name: 'audio.cancel' }));
        expect(transcribe.mock.calls[0][1].signal.aborted).toBe(true);
        await act(async () => { finish(new Response('{"text":"late"}')); });
        expect(URL.createObjectURL).not.toHaveBeenCalled();
        expect(screen.queryByLabelText('audio.transcript')).toBeNull();
    });

    it('does not offer a successful download when a stream ends before its done event', async () => {
        transcribe.mockResolvedValue((async function* () { yield { type: 'transcript.text.delta', delta: 'partial' }; })());
        render(<ASRPage />); upload(); fireEvent.click(screen.getByLabelText('audio.streaming'));
        fireEvent.click(screen.getByRole('button', { name: 'audio.recognize' }));
        expect(await screen.findByRole('alert')).toHaveTextContent('audio.incompleteTranscript');
        expect(screen.queryByRole('link', { name: 'audio.downloadTranscript' })).toBeNull();
    });

    it('blocks oversized input and displays upstream errors without a download', async () => {
        render(<ASRPage />);
        const file = new File(['a'], 'huge.wav'); Object.defineProperty(file, 'size', { value: 25*1024*1024+1 });
        fireEvent.change(screen.getByLabelText('audio.upload'), { target: { files: [file] } });
        expect(screen.getByRole('alert')).toHaveTextContent('audio.fileTooLarge');
        expect(screen.getByRole('button', { name: 'audio.recognize' })).toBeDisabled();
        transcribe.mockReturnValue({ asResponse: () => Promise.reject(new Error('Unsupported model')) });
        upload(); fireEvent.click(screen.getByRole('button', { name: 'audio.recognize' }));
        expect(await screen.findByRole('alert')).toHaveTextContent('Unsupported model');
        expect(screen.queryByRole('link', { name: 'audio.downloadTranscript' })).toBeNull();
    });
});
