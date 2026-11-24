import React, { useCallback, useEffect, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import {
    DndContext,
    KeyboardSensor,
    PointerSensor,
    closestCenter,
    useSensor,
    useSensors,
    type Announcements,
    type DragEndEvent,
} from '@dnd-kit/core';
import { SortableContext, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { Box, IconButton, Tooltip } from '@mui/material';
import { DragIndicator } from '@/components/icons';
import type { Rule } from '@/components/RoutingGraphTypes';
import { moveBefore } from './ruleOrder';

const ruleName = (rule: Rule | undefined) => rule?.request_model || rule?.uuid || '';

interface SortableRuleItemProps {
    rule: Rule;
    itemRef?: React.Ref<HTMLDivElement> | null;
    children: (dragHandle: React.ReactNode) => React.ReactNode;
}

const SortableRuleItem: React.FC<SortableRuleItemProps> = ({ rule, itemRef, children }) => {
    const { t } = useTranslation();
    const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } = useSortable({ id: rule.uuid });

    const label = t('templatePage.reorder.handle', { name: ruleName(rule) });
    const handle = (
        <Tooltip title={label}>
            <IconButton
                ref={setActivatorNodeRef}
                size="small"
                data-rule-drag-handle
                aria-label={label}
                {...attributes}
                {...listeners}
                sx={{ cursor: isDragging ? 'grabbing' : 'grab', touchAction: 'none', color: 'text.secondary' }}
            >
                <DragIndicator fontSize="small" />
            </IconButton>
        </Tooltip>
    );

    return (
        <div ref={itemRef}>
            <Box
                ref={setNodeRef}
                data-dragging={isDragging || undefined}
                sx={{
                    transform: CSS.Transform.toString(transform),
                    transition,
                    position: 'relative',
                    zIndex: isDragging ? 'modal' : 'auto',
                    opacity: isDragging ? 0.85 : 1,
                    // The handle shows on hover/focus, and always on touch (no hover there).
                    '& [data-rule-drag-handle]': { opacity: 0, transition: 'opacity 0.15s' },
                    '&:hover [data-rule-drag-handle], & [data-rule-drag-handle]:focus-visible, &[data-dragging] [data-rule-drag-handle]': { opacity: 1 },
                    '@media (hover: none)': { '& [data-rule-drag-handle]': { opacity: 1 } },
                }}
            >
                {children(handle)}
            </Box>
        </div>
    );
};

export interface SortableRuleListProps {
    rules: Rule[];
    /** Off: plain list, no handles (name-sorted view, slot pages). */
    enabled: boolean;
    /** Rule that gets the scroll anchor ref (new-rule / last-rule), if any. */
    anchorUuid?: string;
    anchorRef?: React.Ref<HTMLDivElement> | null;
    /** The full new uuid order after a drop, and the rule that was moved. */
    onReorder: (orderedUuids: string[], movedUuid: string) => void;
    renderRule: (rule: Rule, dragHandle: React.ReactNode | undefined) => React.ReactNode;
}

export const SortableRuleList: React.FC<SortableRuleListProps> = ({ rules, enabled, anchorUuid, anchorRef, onReorder, renderRule }) => {
    const { t } = useTranslation();
    const sensors = useSensors(
        useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
        useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
    );

    // A refetch can replace the list mid-drag: resolve the drop against the
    // latest order, not the one the gesture started with.
    const latestRules = useRef(rules);
    useEffect(() => { latestRules.current = rules; }, [rules]);

    const ids = useMemo(() => rules.map(r => r.uuid), [rules]);

    const handleDragEnd = useCallback(({ active, over }: DragEndEvent) => {
        if (!over) return;
        const next = moveBefore(latestRules.current.map(r => r.uuid), String(active.id), String(over.id));
        if (next) onReorder(next, String(active.id));
    }, [onReorder]);

    const announcements = useMemo<Announcements>(() => {
        const find = (id: string | number) => latestRules.current.find(r => r.uuid === id);
        const position = (id: string | number) => latestRules.current.findIndex(r => r.uuid === id) + 1;
        const total = () => latestRules.current.length;
        return {
            onDragStart: ({ active }) => t('templatePage.reorder.announce.start', { name: ruleName(find(active.id)), position: position(active.id), total: total() }),
            onDragOver: ({ active, over }) => over
                ? t('templatePage.reorder.announce.over', { name: ruleName(find(active.id)), position: position(over.id), total: total() })
                : undefined,
            onDragEnd: ({ active, over }) => over
                ? t('templatePage.reorder.announce.end', { name: ruleName(find(active.id)), position: position(over.id), total: total() })
                : t('templatePage.reorder.announce.cancel', { name: ruleName(find(active.id)) }),
            onDragCancel: ({ active }) => t('templatePage.reorder.announce.cancel', { name: ruleName(find(active.id)) }),
        };
    }, [t]);

    if (!enabled) {
        return <>{rules.map(rule => (
            <div key={rule.uuid} ref={rule.uuid === anchorUuid ? anchorRef : null}>{renderRule(rule, undefined)}</div>
        ))}</>;
    }

    return (
        <DndContext
            sensors={sensors}
            collisionDetection={closestCenter}
            onDragEnd={handleDragEnd}
            accessibility={{
                announcements,
                screenReaderInstructions: { draggable: t('templatePage.reorder.instructions') },
            }}
        >
            <SortableContext items={ids} strategy={verticalListSortingStrategy}>
                {rules.map(rule => (
                    <SortableRuleItem key={rule.uuid} rule={rule} itemRef={rule.uuid === anchorUuid ? anchorRef : null}>
                        {(handle) => renderRule(rule, handle)}
                    </SortableRuleItem>
                ))}
            </SortableContext>
        </DndContext>
    );
};
