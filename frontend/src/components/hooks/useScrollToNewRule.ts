import { useRef, useState, useEffect, useCallback } from 'react';

export interface UseScrollToNewRuleParams {
    rules: any[];
}

export interface UseScrollToNewRuleReturn {
    scrollContainerRef: React.RefObject<HTMLDivElement>;
    lastRuleRef: React.RefObject<HTMLDivElement>;
    newRuleUuid: string | null;
    setNewRuleUuid: (uuid: string | null) => void;
}

export const useScrollToNewRule = ({
    rules,
}: UseScrollToNewRuleParams): UseScrollToNewRuleReturn => {
    const scrollContainerRef = useRef<HTMLDivElement>(null);
    const lastRuleRef = useRef<HTMLDivElement>(null);
    const [newRuleUuid, setNewRuleUuid] = useState<string | null>(null);

    // Scroll to new rule when it's created
    useEffect(() => {
        if (newRuleUuid && lastRuleRef.current && scrollContainerRef.current) {
            const container = scrollContainerRef.current;
            const target = lastRuleRef.current;

            // Calculate the scroll position to show the target at the top of the container
            const scrollTop = target.offsetTop - container.offsetTop;

            container.scrollTo({
                top: scrollTop,
                behavior: 'smooth'
            });

            // Clear the new rule UUID after scrolling
            setNewRuleUuid(null);
        }
    }, [newRuleUuid]);

    return {
        scrollContainerRef,
        lastRuleRef,
        newRuleUuid,
        setNewRuleUuid,
    };
};
