import {useEffect, useRef, useState} from 'react';
import {runProbe} from '@/components/probe/runProbe';
import {notify} from '@/utils/notify';
import type {ConfigProvider, ConfigRecord} from '@/components/RoutingGraphTypes';

export interface UseCodexResponsesToggleOptions {
    record: ConfigRecord;
    /** The rule's highest-priority active service, or undefined if it has none. */
    primaryService: ConfigProvider | undefined;
    onUpdateRecord?: (field: keyof ConfigRecord, value: any) => void;
}

export interface UseCodexResponsesToggleResult {
    enabled: boolean;
    probing: boolean;
    onToggle: () => void;
}

// probeResponsesSupport runs the direct, real-upstream capability check the
// toggle needs before trusting a provider/model with /responses traffic —
// shared by the toggle handler and the revalidation effect below so the
// request shape lives in exactly one place.
async function probeResponsesSupport(service: ConfigProvider) {
    return runProbe({
        target_type: 'provider',
        provider_uuid: service.provider,
        model: service.model,
        test_mode: 'simple',
        direct: true,
        endpoint: 'responses',
    });
}

// useCodexResponsesToggle owns the "native OpenAI Responses API" switch shown
// on Codex-scenario rule cards: a pre-flight probe against the real upstream
// before the flag is set, and automatic re-validation if the rule's bound
// provider/model changes mid-session.
export function useCodexResponsesToggle({
    record,
    primaryService,
    onUpdateRecord,
}: UseCodexResponsesToggleOptions): UseCodexResponsesToggleResult {
    const [probing, setProbing] = useState(false);
    const enabled = record.flags?.openaiEndpointOverride === 'responses';

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

        if (!enabled || !primaryService) return;

        let cancelled = false;
        setProbing(true);
        const revertWithNotice = (message: string) => {
            if (cancelled) return;
            onUpdateRecord?.('flags', {...record.flags, openaiEndpointOverride: 'auto'});
            notify.error(message, {title: 'Responses API disabled'});
        };
        probeResponsesSupport(primaryService).then((result) => {
            if (!result.success) {
                revertWithNotice(
                    `The provider/model for this rule changed and no longer supports the Responses API — reverted to Chat Completions. (${result.error?.message || 'check failed'})`,
                );
            }
        }).catch(() => {
            // Fail closed: don't leave a possibly-broken override silently
            // forcing traffic at a 404 just because the re-check itself failed.
            revertWithNotice('Could not re-verify Responses API support after the model changed — reverted to Chat Completions.');
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

    const onToggle = async () => {
        if (!primaryService) return;

        // Disabling never needs a probe — it's just reverting to the
        // conservative default, always safe.
        if (enabled) {
            onUpdateRecord?.('flags', {...record.flags, openaiEndpointOverride: 'auto'});
            return;
        }

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

    return {enabled, probing, onToggle};
}
