import {useEffect, useRef, useState} from 'react';
import {runProbe} from '@/components/probe/runProbe';
import {notify} from '@/utils/notify';
import type {ConfigProvider, ConfigRecord} from '@/components/RoutingGraphTypes';

export interface UseResponsesToggleOptions {
    record: ConfigRecord;
    /** The rule's highest-priority active service, or undefined if it has none. */
    primaryService: ConfigProvider | undefined;
    onUpdateRecord?: (field: keyof ConfigRecord, value: any) => void;
}

export interface UseResponsesToggleResult {
    selection: OpenAIEndpointSelection;
    probing: boolean;
    onSelect: (selection: OpenAIEndpointSelection) => Promise<void>;
}

export type OpenAIEndpointSelection = 'auto' | 'chat' | 'responses';

// probeResponsesSupport runs the direct, real-upstream capability check the
// toggle needs before trusting a provider/model with /responses traffic —
// shared by the toggle handler and the revalidation effect below so the
// request shape lives in exactly one place.
async function probeResponsesSupport(service: ConfigProvider) {
    return runProbe({
        target_type: 'provider',
        provider_uuid: service.provider,
        model: service.model,
        stream: false,
        direct: true,
        protocol: 'openai_responses',
    });
}

// useResponsesToggle owns the OpenAI upstream endpoint choice shown on
// any rule card whose primary provider is OpenAI-style — it's the rule-level
// `openaiEndpointOverride` flag (see .design/openai-endpoint-routing.md §3,
// Layer 2), which is per-rule and provider-agnostic by design, not a
// Codex-only feature. A pre-flight probe against the real upstream gates the
// flag before it's set, and the hook auto-revalidates if the rule's bound
// provider/model changes mid-session.
export function useResponsesToggle({
    record,
    primaryService,
    onUpdateRecord,
}: UseResponsesToggleOptions): UseResponsesToggleResult {
    const [probing, setProbing] = useState(false);
    const rawSelection = record.flags?.openaiEndpointOverride;
    const selection: OpenAIEndpointSelection = rawSelection === 'chat' || rawSelection === 'responses' ? rawSelection : 'auto';

    // The pre-flight probe only validates the provider+model bound *at toggle
    // time*. If the user later swaps the rule's provider/model (drag a new
    // provider in, edit the service, change tier, …), the flag survives
    // untouched — but the resolver always honors a rule-flag override with no
    // silent downgrade (see .design/openai-endpoint-routing.md §4.1), so a
    // now-unsupported provider would hard-fail every request on /responses
    // while the switch still shows "on". Re-validate whenever the bound
    // provider/model actually changes mid-session and the flag is currently
    // set; auto-revert + notify on failure, stay silent on success.
    const primaryServiceKey = primaryService ? `${primaryService.provider}:${primaryService.model}` : null;
    const lastCheckedServiceKeyRef = useRef(primaryServiceKey);
    useEffect(() => {
        // Ref starts equal to the initial key, so this only fires on an
        // actual change during the session — never on mount.
        if (lastCheckedServiceKeyRef.current === primaryServiceKey) return;
        lastCheckedServiceKeyRef.current = primaryServiceKey;

        if (selection !== 'responses' || !primaryService) return;

        let cancelled = false;
        setProbing(true);
        const revertWithNotice = (message: string) => {
            if (cancelled) return;
            onUpdateRecord?.('flags', {...record.flags, openaiEndpointOverride: 'auto'});
            notify.error(message, {title: 'Endpoint returned to Auto'});
        };
        probeResponsesSupport(primaryService).then((result) => {
            if (!result.success) {
                revertWithNotice(
                    `The provider/model for this rule changed and no longer supports the Responses API — returned to Auto. (${result.error?.message || 'check failed'})`,
                );
            }
        }).catch(() => {
            // Fail closed: don't leave a possibly-broken override silently
            // forcing traffic at a 404 just because the re-check itself failed.
            revertWithNotice('Could not re-verify Responses API support after the model changed — returned to Auto.');
        }).finally(() => {
            if (!cancelled) setProbing(false);
        });

        return () => {
            cancelled = true;
        };
        // Deliberately re-runs only on provider/model change, not on every
        // record.flags update (including the one this effect itself makes).
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [primaryServiceKey]);

    const onSelect = async (nextSelection: OpenAIEndpointSelection) => {
        if (nextSelection === selection || probing) return;

        // Auto follows the model catalog and provider declaration; forcing
        // Chat is also an immediate, explicit choice. Neither needs a probe.
        if (nextSelection !== 'responses') {
            onUpdateRecord?.('flags', {...record.flags, openaiEndpointOverride: nextSelection});
            return;
        }

        if (!primaryService) return;

        setProbing(true);
        try {
            const result = await probeResponsesSupport(primaryService);
            if (result.success) {
                onUpdateRecord?.('flags', {...record.flags, openaiEndpointOverride: 'responses'});
                notify.success('Native Responses API enabled for this rule.');
            } else {
                notify.error(
                    result.error?.message || 'This provider/model does not appear to support the Responses API.',
                    {title: 'Responses API check failed'},
                );
            }
        } catch (err: any) {
            notify.error(err?.message || 'Failed to check Responses API support.', {title: 'Responses API check failed'});
        } finally {
            setProbing(false);
        }
    };

    return {selection, probing, onSelect};
}
