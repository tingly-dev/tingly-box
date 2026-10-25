import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

export const MAX_AUDIO_UPLOAD_BYTES = 25 * 1024 * 1024;

export function useAudioRecorder(onFile: (file: File) => void) {
    const { t } = useTranslation();
    const [recording, setRecording] = useState(false);
    const [starting, setStarting] = useState(false);
    const [error, setError] = useState('');
    const recorder = useRef<MediaRecorder | null>(null);
    const stream = useRef<MediaStream | null>(null);
    const generation = useRef(0);
    const timeout = useRef<ReturnType<typeof setTimeout> | null>(null);
    const release = useCallback(() => {
        stream.current?.getTracks().forEach(track => track.stop());
        stream.current = null;
        if (timeout.current) clearTimeout(timeout.current);
        timeout.current = null;
    }, []);
    useEffect(() => () => {
        generation.current++;
        if (recorder.current?.state === 'recording') recorder.current.stop();
        release();
    }, [release]);
    const start = async () => {
        if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined') {
            setError(t('audio.recordingUnavailable')); return;
        }
        const current = ++generation.current;
        setStarting(true); setError('');
        let captured: MediaStream | null = null;
        try {
            captured = await navigator.mediaDevices.getUserMedia({ audio: true });
            if (current !== generation.current) { captured.getTracks().forEach(track => track.stop()); return; }
            stream.current = captured;
            const mimeType = ['audio/webm;codecs=opus', 'audio/mp4', 'audio/ogg;codecs=opus'].find(type => MediaRecorder.isTypeSupported(type));
            const active = new MediaRecorder(captured, mimeType ? { mimeType } : undefined);
            recorder.current = active;
            const chunks: Blob[] = [];
            let size = 0;
            active.ondataavailable = event => {
                if (!event.data.size) return;
                size += event.data.size;
                if (size <= MAX_AUDIO_UPLOAD_BYTES) chunks.push(event.data);
                else if (active.state === 'recording') active.stop();
            };
            active.onerror = () => {
                generation.current++;
                release(); setRecording(false); setError(t('audio.recordingFailed'));
            };
            active.onstop = () => {
                if (current !== generation.current) { captured?.getTracks().forEach(track => track.stop()); return; }
                release();
                setRecording(false);
                if (size > MAX_AUDIO_UPLOAD_BYTES) { setError(t('audio.fileTooLarge')); return; }
                if (!size) { setError(t('audio.emptyRecording')); return; }
                const type = active.mimeType || chunks[0]?.type || 'audio/webm';
                const extension = type.includes('mp4') ? 'm4a' : type.includes('ogg') ? 'ogg' : 'webm';
                onFile(new File(chunks, `recording.${extension}`, { type }));
            };
            active.start(1000);
            setRecording(true);
            timeout.current = setTimeout(() => { if (active.state === 'recording') active.stop(); }, 10 * 60 * 1000);
        } catch (cause) {
            captured?.getTracks().forEach(track => track.stop());
            if (current === generation.current) { release(); setError(cause instanceof Error ? cause.message : t('audio.recordingFailed')); }
        } finally {
            if (current === generation.current) setStarting(false);
        }
    };
    const stop = () => { if (recorder.current?.state === 'recording') recorder.current.stop(); };
    return { recording, starting, error, start, stop };
}
