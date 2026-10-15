import { useCallback, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api, unwrap } from '../client';
import { useNotify } from './useNotify';
import { readLocal, writeLocal } from '../utils/storage';
import { useAppData } from '../contexts/AppDataContext';
import { useModels } from './useModels';

/** Router state ChatPage picks up to send the first message. */
export interface PendingFirstMessage {
    pending: { text: string; model: string };
}

/**
 * Creates a conversation and opens it; ChatPage sends the text as the first
 * message so the user lands directly in the streaming reply.
 */
export function useStartChat() {
    const navigate = useNavigate();
    const notify = useNotify();
    return useCallback(
        async (opts: { text: string; model: string }) => {
            try {
                const conv = await unwrap(api.POST('/api/v1/conversations', { body: { model: opts.model || undefined } }));
                const state: PendingFirstMessage = { pending: { text: opts.text, model: opts.model } };
                navigate(`/chat/${conv.id}`, { state });
                return true;
            } catch (e) {
                notify((e as Error).message, 'error');
                return false;
            }
        },
        [navigate, notify],
    );
}

/**
 * The model new messages go to, always a concrete ID (never an alias):
 * explicitly picked here → last picked in this browser → configured default →
 * first model the endpoint lists. `setModel` remembers the pick for next
 * time; `adoptModel` follows a conversation's model without changing that
 * remembered preference.
 */
export function useChosenModel() {
    const { settings } = useAppData();
    const { models } = useModels();
    const [picked, setPicked] = useState<string | null>(null);
    const model = picked || readLocal('lastModel') || settings?.default_model || models[0]?.id || '';
    const setModel = useCallback((m: string) => {
        setPicked(m);
        writeLocal('lastModel', m);
    }, []);
    const adoptModel = useCallback((m: string) => setPicked(m), []);
    return { model, setModel, adoptModel };
}
