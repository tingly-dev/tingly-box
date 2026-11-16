import {useEffect, useMemo, useState} from 'react';
import type {Thread, ThreadEvent} from '@tingly/projects';
import RouteStatusLine from '@/components/claudeCode/RouteStatusLine';
import {sessionUsage} from '@/components/claudeCode/sessionUsage';
import {profileApi, type ClaudeCodeRoute} from '@/services/profileApi';

const parse = (payload?: string): unknown => {
    try {
        return payload ? JSON.parse(payload) : undefined;
    } catch {
        return undefined;
    }
};

// SessionStatusLine is a folder session's status line under its message box:
// token figures from the transcript's usage events, routing and quota from
// the claude_code route of the session's profile and tier, re-read as each
// turn ends or the session's launch changes.
const SessionStatusLine = ({session, events}: {session: Thread; events: ThreadEvent[]}) => {
    const usageEvents = useMemo(() => events.filter((e) => e.kind === 'usage').map((e) => ({kind: e.kind, payload: parse(e.payload)})), [events]);
    const usage = useMemo(() => sessionUsage(usageEvents), [usageEvents]);
    const turns = usageEvents.length;
    const lastModel = usage?.latest.model;
    const [status, setStatus] = useState<ClaudeCodeRoute>();
    const profile = session.profile ?? '';
    const tier = session.model ?? '';

    useEffect(() => {
        let live = true;
        profileApi.getClaudeCodeRoute({profile, tier, model: lastModel})
            .then((s) => live && setStatus(s))
            .catch(() => live && setStatus(undefined));
        return () => {
            live = false;
        };
    }, [profile, tier, lastModel, turns]);

    return <RouteStatusLine status={status} usage={usage}/>;
};

export default SessionStatusLine;
