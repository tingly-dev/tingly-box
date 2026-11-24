import {useEffect, useState} from 'react';
import {api} from '@/services/api';
import {compactLimits} from './LimitsEditor';
import {EMPTY_POLICY_DRAFT, type PolicyDraft} from './PolicyLimitsFields';

export type PolicyKind = 'user' | 'team' | 'team_member' | 'model';

/** Loads one limit policy into an editable draft while `active`, and saves it back. */
export function usePolicyDraft(kind: PolicyKind, ref: string | undefined, active: boolean) {
    const [draft, setDraft] = useState<PolicyDraft>(EMPTY_POLICY_DRAFT);
    const [loadedKey, setLoadedKey] = useState<string | null>(null);
    const key = `${kind}:${ref ?? ''}`;

    useEffect(() => {
        if (!active || !ref) return;
        let cancelled = false;
        api.getLimitPolicies({kind, ref}).then((res: any) => {
            if (cancelled) return;
            const p = res?.data?.[0];
            setDraft(p ? {
                limits: p.limits ?? {},
                overflow: p.overflow === 'reject' ? 'reject' : 'queue',
                queue_timeout_seconds: p.queue_timeout_seconds || undefined,
            } : EMPTY_POLICY_DRAFT);
            setLoadedKey(key);
        }).catch(() => {
            if (!cancelled) setLoadedKey(key);
        });
        return () => {
            cancelled = true;
        };
    }, [active, kind, ref, key]);

    const loading = active && !!ref && loadedKey !== key;

    /** Saves the draft; an emptied editor deletes the policy server-side. */
    const save = async () => {
        if (!ref) return {success: false as const, error: 'missing ref'};
        return api.putLimitPolicy(kind, ref, {
            limits: compactLimits(draft.limits),
            overflow: draft.overflow,
            queue_timeout_seconds: draft.queue_timeout_seconds,
        });
    };

    return {draft, setDraft, loading, save};
}
