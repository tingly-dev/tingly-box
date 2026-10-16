import * as deskApi from '@/services/deskApi';
import type {MessageInfo, SessionStatus} from '@/services/deskApi';
import {useEffect, useMemo, useState} from 'react';
import RouteStatusLine from '@/components/claudeCode/RouteStatusLine';
import {sessionUsage} from '@/components/claudeCode/sessionUsage';

interface StatusLineProps {
    sessionId: string;
    // The session's profile decides the scenario and its tier the model, so
    // a switch re-resolves the route and quota before the next turn.
    profile: string;
    model: string;
    messages: MessageInfo[];
}

// StatusLine is a Desk session's status line: token figures from the
// transcript's usage entries, routing and quota from the session status
// endpoint, refreshed as each turn ends.
const StatusLine = ({sessionId, profile, model: tier, messages}: StatusLineProps) => {
    const usage = useMemo(() => sessionUsage(messages), [messages]);
    const turns = useMemo(() => messages.filter((m) => m.kind === 'usage').length, [messages]);
    const [status, setStatus] = useState<SessionStatus>();

    useEffect(() => {
        let live = true;
        deskApi.getStatus(sessionId).then((s) => live && setStatus(s)).catch(() => live && setStatus(undefined));
        return () => {
            live = false;
        };
    }, [sessionId, profile, tier, turns]);

    return <RouteStatusLine status={status} usage={usage}/>;
};

export default StatusLine;
