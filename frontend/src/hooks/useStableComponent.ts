import { useMemo, useRef } from 'react';
import type React from 'react';

/**
 * Turns a render function that closes over a hook's state into a component
 * whose identity never changes.
 *
 * Hooks like useModelSelectDialog hand their caller a ready-made dialog
 * component. Declaring it inline (`const Dialog: React.FC = () => ...`) makes a
 * brand-new component type on every render of the host page, so React unmounts
 * and remounts the whole subtree each time the page re-renders (a status poll is
 * enough): the model list empties and refetches, and the open tab, search and
 * page are lost. The returned component stays the same; each render reads the
 * latest render function, so it still sees the hook's current state.
 */
export function useStableComponent<P extends object>(
    render: (props: P) => React.ReactElement | null,
): React.FC<P> {
    const renderRef = useRef(render);
    renderRef.current = render;
    return useMemo(() => {
        const StableComponent: React.FC<P> = (props) => renderRef.current(props);
        return StableComponent;
    }, []);
}
