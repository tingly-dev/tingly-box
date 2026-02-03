import { useEffect, useState, RefObject } from 'react';

/**
 * Custom hook to measure and track the height of a header element.
 * Uses ResizeObserver to detect size changes and updates accordingly.
 *
 * @param headerRef - React ref object pointing to the header element
 * @param shouldMeasure - Condition to determine when to start measuring (e.g., when providers are loaded)
 * @param deps - Additional dependencies that trigger re-measurement
 * @returns The current height of the header element in pixels
 */
export function useHeaderHeight(
    headerRef: RefObject<HTMLElement>,
    shouldMeasure: boolean,
    deps: React.DependencyList = []
): number {
    const [height, setHeight] = useState<number>(0);

    useEffect(() => {
        if (!shouldMeasure || !headerRef.current) {
            return;
        }

        // Small delay to ensure DOM is fully rendered
        const timeoutId = setTimeout(() => {
            if (!headerRef.current) {
                return;
            }

            const updateHeight = () => {
                if (headerRef.current) {
                    const h = headerRef.current.offsetHeight || 0;
                    setHeight(h);
                }
            };

            updateHeight();

            const resizeObserver = new ResizeObserver(() => {
                updateHeight();
            });

            resizeObserver.observe(headerRef.current);

            return () => {
                resizeObserver.disconnect();
            };
        }, 200);

        return () => {
            clearTimeout(timeoutId);
        };
    }, [shouldMeasure, headerRef, ...deps]);

    return height;
}

export default useHeaderHeight;
