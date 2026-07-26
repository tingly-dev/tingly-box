import {api} from '@/services/api';
import {useCallback, useState} from 'react';

// useChatProbe is the chat-capability probe runner. For a given (bot, chat),
// it fires one capability (notify / confirm) end-to-end and normalizes the
// outcome into a single ChatProbeResult — so the UI only renders one shape,
// mirroring how the probe feature folds everything into one ProbeResult envelope
// (see components/probe/runProbe.ts).
//
// - notify  → api.notifyBot; success = delivered (200 {ok:true}).
// - confirm → api.interactBot then long-polls api.waitBotInteract, retrying on
//   `pending` (504) until the prompt is answered/cancelled/timed out/expired.
//   success = answered; decision.selected ∈ {allow, deny}.
//
// choose/ask share the confirm runner shape but are gated at the button layer
// (their backend option/free-text threading into the IM keyboard is incomplete),
// so this hook doesn't expose them yet.

export type ChatCapability = 'notify' | 'confirm';

export interface ChatProbeResult {
    capability: ChatCapability;
    /** true only for a clean success: notify delivered, or confirm answered. */
    success: boolean;
    /** One-glance status label: delivered | answered | cancelled | timed-out | expired | failed. */
    status: 'delivered' | 'answered' | 'cancelled' | 'timed-out' | 'expired' | 'failed';
    latencyMs: number;
    /** The reply decision (confirm: {selected:'allow'|'deny'}), if any. */
    decision?: Record<string, unknown>;
    /** Error/reason text on non-success. */
    error?: string;
    /** Backend reason (e.g. why a prompt timed out), if any. */
    reason?: string;
    /** Raw payload for the "Raw JSON" collapsible. */
    raw?: unknown;
}

type ResultMap = Record<string, ChatProbeResult | undefined>;

const key = (chatID: string, capability: ChatCapability) => `${chatID}:${capability}`;

// Confirm long-poll budget: keep polling until the prompt resolves or expires.
// Each wait call is ~45s (server max 50s); cap the whole thing so it can't run
// forever if something goes wrong.
const CONFIRM_TOTAL_BUDGET_MS = 6 * 60 * 1000;
const WAIT_TIMEOUT_MS = 45_000;

export interface UseChatProbeResult {
    results: ResultMap;
    running: Record<string, boolean>;
    run: (botUUID: string, chatID: string, capability: ChatCapability) => Promise<void>;
    isRunning: (chatID: string, capability: ChatCapability) => boolean;
    getResult: (chatID: string, capability: ChatCapability) => ChatProbeResult | undefined;
    clear: (chatID: string, capability: ChatCapability) => void;
}

export function useChatProbe(): UseChatProbeResult {
    const [results, setResults] = useState<ResultMap>({});
    const [running, setRunning] = useState<Record<string, boolean>>({});

    const setResult = useCallback((k: string, r: ChatProbeResult) => {
        setResults((prev) => ({...prev, [k]: r}));
    }, []);
    const setRun = useCallback((k: string, v: boolean) => {
        setRunning((prev) => ({...prev, [k]: v}));
    }, []);

    const runNotify = useCallback(async (botUUID: string, chatID: string, started: number): Promise<ChatProbeResult> => {
        const result = await api.notifyBot(botUUID, {
            chat_id: chatID,
            title: 'Test notification',
            body: 'Sent from the IM Notify probe — this verifies one-way delivery to this chat.',
            level: 'info',
        });
        const latencyMs = Date.now() - started;
        if (result.error) {
            return {capability: 'notify', success: false, status: 'failed', latencyMs, error: result.error, raw: result};
        }
        return {capability: 'notify', success: true, status: 'delivered', latencyMs, raw: result};
    }, []);

    const runConfirm = useCallback(async (botUUID: string, chatID: string, started: number): Promise<ChatProbeResult> => {
        const start = await api.interactBot(botUUID, {
            chat_id: chatID,
            kind: 'confirm',
            title: 'Test: approve?',
            body: 'This verifies the bot\'s interactive prompt works. Tap Allow or Deny.',
            // confirm requires ≥1 option (handler validates len>0 before the
            // channel renders). The channel's permission path renders
            // Allow/Deny/Always-Allow regardless, and returns decision.selected
            // ∈ {allow, deny, always} — so send those values.
            options: [
                {value: 'allow', label: 'Allow', style: 'primary'},
                {value: 'deny', label: 'Deny', style: 'danger'},
            ],
            timeout_seconds: 120,
        });
        if (start.error || !start.request_id) {
            return {capability: 'confirm', success: false, status: 'failed', latencyMs: Date.now() - started, error: start.error || 'no request_id', raw: start};
        }

        // Long-poll loop: retry on pending until resolved/expired/budget hit.
        const deadline = started + CONFIRM_TOTAL_BUDGET_MS;
        let last: {status?: string; decision?: Record<string, unknown>; reason?: string; error?: string} = {status: 'pending'};
        while (Date.now() < deadline) {
            last = await api.waitBotInteract(botUUID, start.request_id, WAIT_TIMEOUT_MS);
            if (last.error) {
                return {capability: 'confirm', success: false, status: 'failed', latencyMs: Date.now() - started, error: last.error, raw: {start, ...last}};
            }
            if (last.status === 'answered') {
                return {capability: 'confirm', success: true, status: 'answered', latencyMs: Date.now() - started, decision: last.decision, raw: {start, ...last}};
            }
            if (last.status === 'cancelled') {
                return {capability: 'confirm', success: false, status: 'cancelled', latencyMs: Date.now() - started, decision: last.decision, raw: {start, ...last}};
            }
            if (last.status === 'timeout' || last.status === 'error') {
                return {capability: 'confirm', success: false, status: 'timed-out', latencyMs: Date.now() - started, decision: last.decision, reason: last.reason, raw: {start, ...last}};
            }
            if (last.status === 'expired' || last.status === 'unavailable') {
                return {capability: 'confirm', success: false, status: 'expired', latencyMs: Date.now() - started, raw: {start, ...last}};
            }
            // status === 'pending' → loop and long-poll again
        }
        return {capability: 'confirm', success: false, status: 'timed-out', latencyMs: Date.now() - started, raw: {start, ...last}};
    }, []);

    const run = useCallback(async (botUUID: string, chatID: string, capability: ChatCapability) => {
        const k = key(chatID, capability);
        setRun(k, true);
        const started = Date.now();
        try {
            const r = capability === 'notify'
                ? await runNotify(botUUID, chatID, started)
                : await runConfirm(botUUID, chatID, started);
            setResult(k, r);
        } finally {
            setRun(k, false);
        }
    }, [runNotify, runConfirm, setRun, setResult]);

    const isRunning = useCallback((chatID: string, capability: ChatCapability) => Boolean(running[key(chatID, capability)]), [running]);
    const getResult = useCallback((chatID: string, capability: ChatCapability) => results[key(chatID, capability)], [results]);
    const clear = useCallback((chatID: string, capability: ChatCapability) => {
        const k = key(chatID, capability);
        setResults((prev) => {
            const next = {...prev};
            delete next[k];
            return next;
        });
    }, []);

    return {results, running, run, isRunning, getResult, clear};
}

export default useChatProbe;
