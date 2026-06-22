import {api} from '../../services/api';

export interface VerificationResult {
    success: boolean;
    message: string;
    details?: string;
    responseTime?: number;
    modelsCount?: number;
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
}

// Runs the lightweight provider probe and shapes the result into the
// {success,message,details,...} record the dialog renders. Pure I/O — does
// not touch React state — so it stays out of the component file.
export const runProviderProbe = async (
    params: RunProbeParams,
    messages: ProbeMessages,
): Promise<VerificationResult> => {
    try {
        const result = await api.probeProviderLightweight(
            params.name,
            params.apiStyle,
            params.apiBase,
            params.token,
            params.authType,
        );

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
        return {
            success: false,
            message: messages.networkError,
        };
    }
};
