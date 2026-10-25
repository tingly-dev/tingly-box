import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MAX_AUDIO_UPLOAD_BYTES, useAudioRecorder } from './useAudioRecorder';

let latest: FakeRecorder;
class FakeRecorder {
    static isTypeSupported = () => true;
    state = 'inactive';
    mimeType = 'audio/webm';
    ondataavailable?: (event: { data: Blob }) => void;
    onstop?: () => void;
    onerror?: () => void;
    constructor() { latest = this; }
    start() { this.state = 'recording'; }
    stop() { this.state = 'inactive'; this.onstop?.(); }
}

describe('microphone recording', () => {
    const getUserMedia = vi.fn();
    const stopTrack = vi.fn();
    const stream = { getTracks: () => [{ stop: stopTrack }] };
    let previous: PropertyDescriptor | undefined;
    beforeEach(() => {
        vi.clearAllMocks();
        previous = Object.getOwnPropertyDescriptor(navigator, 'mediaDevices');
        Object.defineProperty(navigator, 'mediaDevices', { configurable: true, value: { getUserMedia } });
        vi.stubGlobal('MediaRecorder', FakeRecorder);
        getUserMedia.mockResolvedValue(stream);
    });
    afterEach(() => {
        vi.unstubAllGlobals();
        if (previous) Object.defineProperty(navigator, 'mediaDevices', previous);
        else Reflect.deleteProperty(navigator, 'mediaDevices');
    });

    it('returns a recorded file and releases microphone tracks', async () => {
        const onFile = vi.fn();
        const { result } = renderHook(() => useAudioRecorder(onFile));
        await act(() => result.current.start());
        expect(result.current.recording).toBe(true);
        act(() => {
            latest.ondataavailable?.({ data: new Blob(['audio'], { type: 'audio/webm' }) });
            result.current.stop();
        });
        expect(result.current.recording).toBe(false);
        expect(onFile).toHaveBeenCalledWith(expect.any(File));
        expect(onFile.mock.calls[0][0].name).toBe('recording.webm');
        expect(stopTrack).toHaveBeenCalled();
    });

    it('releases a microphone permission result that arrives after unmount', async () => {
        let finish!: (value: typeof stream) => void;
        getUserMedia.mockReturnValue(new Promise(resolve => { finish = resolve; }));
        const onFile = vi.fn();
        const { result, unmount } = renderHook(() => useAudioRecorder(onFile));
        let pending!: Promise<void>;
        act(() => { pending = result.current.start(); });
        unmount();
        await act(async () => { finish(stream); await pending; });
        expect(stopTrack).toHaveBeenCalledOnce();
        expect(onFile).not.toHaveBeenCalled();
    });

    it('stops and rejects oversized recordings', async () => {
        const onFile = vi.fn();
        const { result } = renderHook(() => useAudioRecorder(onFile));
        await act(() => result.current.start());
        act(() => latest.ondataavailable?.({ data: { size: MAX_AUDIO_UPLOAD_BYTES+1 } as Blob }));
        expect(result.current.recording).toBe(false);
        expect(result.current.error).toBe('audio.fileTooLarge');
        expect(onFile).not.toHaveBeenCalled();
        expect(stopTrack).toHaveBeenCalled();
    });
});
