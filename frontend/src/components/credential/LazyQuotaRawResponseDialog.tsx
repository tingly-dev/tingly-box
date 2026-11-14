import { lazy, Suspense, useState } from 'react';
import type { ComponentProps } from 'react';

// The dialog pulls in the syntax highlighter (~0.1 MB), but it is reached from
// the quota cells that ride along with the always-loaded layout. Load it on
// first open instead, and keep it mounted afterwards so closing still animates.
const Dialog = lazy(() =>
    import('./QuotaRawResponseDialog').then((m) => ({ default: m.QuotaRawResponseDialog })),
);

export function QuotaRawResponseDialog(props: ComponentProps<typeof Dialog>) {
    const [wasOpen, setWasOpen] = useState(props.open);
    if (props.open && !wasOpen) setWasOpen(true);
    if (!wasOpen) return null;
    return (
        <Suspense fallback={null}>
            <Dialog {...props} />
        </Suspense>
    );
}
