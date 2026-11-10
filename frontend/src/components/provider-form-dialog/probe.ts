import {api} from '../../services/api';

export interface VerificationResult {
    success: boolean;
    message: string;
    details?: string;
    responseTime?: number;
    modelsCount?: number;
    /** True when the probe ended because it was cancelled (user stop or watchdog timeout). */
    cancelled?: boolean;
}

interface RunProbeParams {
    name: string;
    apiStyle: 'openai' | 'anthropic';
    apiBase: string;
    token: string;
    authType?: 'api_key' | 'oauth';
}

interface ProbeMessages {
    failed: string;
    networkError: string;
    cancelled: string;
    timedOut: string;
}

// Total wall-clock bound for one "Test Connection" run. The backend already
// times out each sub-probe (OPTIONS 5s, models 15s, chat/responses 10s) and
// honours request-context cancellation, but they run sequentially — a fully
// hung upstream can chain to ~40s. The watchdog bounds the whole run and the
// same signal doubles as the user's Cancel button.
export const PROBE_TIMEOUT_MS = 30_000;

export interface ProbeRun {
    /** Abort signal wired into the probe HTTP request(s). */
    signal: AbortSignal;
    /** True once the watchdog fired (as opposed to an explicit user cancel). */
    timedOut: () => boolean;
    /** Abort the in-flight probe — the UI's Stop button. */
    cancel: () => void;
    /** Stop the watchdog timer; call when the probe run settles. */
    dispose: () => void;
}

// Opens a cancellable probe run: one AbortController shared by every probe of
// the run (single- or dual-protocol), with the timeout watchdog pre-armed.
export const beginProbeRun = (): ProbeRun => {
    const controller = new AbortController();
    let timedOut = false;
    const timer = setTimeout(() => {
        timedOut = true;
        controller.abort();
    }, PROBE_TIMEOUT_MS);
    return {
        signal: controller.signal,
        cancel: () => controller.abort(),
        dispose: () => clearTimeout(timer),
        timedOut: () => timedOut,
    };
};

// Runs the lightweight provider probe and shapes the result into the
// {success,message,details,...} record the dialog renders. Pure I/O — does
// not touch React state — so it stays out of the component file.
export const runProviderProbe = async (
    params: RunProbeParams,
    messages: ProbeMessages,
    run?: ProbeRun,
): Promise<VerificationResult> => {
    const signal = run?.signal;
    try {
        const result = await api.probeProviderLightweight(
            params.name,
            params.apiStyle,
            params.apiBase,
            params.token,
            params.authType,
            signal,
        );

        // openapi-fetch resolves (not rejects) on abort — controlApi swallows
        // the fetch error into {success:false,error}. Check the signal before
        // shaping so a cancelled run never masquerades as a failed probe.
        if (signal?.aborted) {
            return cancelledResult(messages, run);
        }

        if (result.success && result.data) {
            const probeData = result.data;
            const isValid = probeData.valid !== false;

            const details: string[] = [];

            if (probeData.options_success) {
                details.push(`✓ OPTIONS (${probeData.options_response_time}ms)`);
            } else if (probeData.options_message) {
                details.push(`✗ OPTIONS: ${probeData.options_message}`);
            }

            if (probeData.models_success) {
                details.push(`✓ Models (${probeData.models_response_time}ms, ${probeData.models_count} models)`);
            } else if (probeData.models_message) {
                details.push(`✗ Models: ${probeData.models_message}`);
            }

            if (probeData.chat_success !== undefined) {
                if (probeData.chat_success) {
                    details.push(`✓ Chat (${probeData.chat_response_time}ms)`);
                } else if (probeData.chat_message) {
                    details.push(`✗ Chat: ${probeData.chat_message}`);
                }
            }

            if (probeData.responses_success !== undefined) {
                if (probeData.responses_success) {
                    details.push(`✓ Responses (${probeData.responses_response_time}ms)`);
                } else if (probeData.responses_message) {
                    details.push(`✗ Responses: ${probeData.responses_message}`);
                }
            }

            if (probeData.warning) {
                details.push(`⚠ ${probeData.warning}`);
            }

            return {
                success: isValid,
                message: probeData.message,
                details: details.join(' • '),
                responseTime: probeData.options_response_time || probeData.models_response_time,
                modelsCount: probeData.models_count,
            };
        }

        return {
            success: false,
            message: result.error?.message || messages.failed,
        };
    } catch (_error) {
        if (signal?.aborted) {
            return cancelledResult(messages, run);
        }
        return {
            success: false,
            message: messages.networkError,
        };
    }
};

const cancelledResult = (messages: ProbeMessages, run?: ProbeRun): VerificationResult => ({
    success: false,
    cancelled: true,
    message: run?.timedOut() ? messages.timedOut : messages.cancelled,
});
