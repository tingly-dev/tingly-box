import { ConfigRow } from './ConfigRow';
import React, { type ReactNode } from 'react';

// ============================================================================
// Types
// ============================================================================

interface UnifiedConfigRowProps {
    /** Label text (optional - when omitted, only content is shown) */
    label?: string;
    /** Content to display (usually clickable text) */
    children: ReactNode;
    /** Action buttons on the right */
    actions?: ReactNode;
    /** Maximum width of the row (default: 700) */
    maxWidth?: number;
    /** Optional info tooltip for the label */
    labelTooltip?: string;
}

// ============================================================================
// Component
// ============================================================================

/**
 * Unified configuration row component (legacy wrapper).
 * Delegates to ConfigRow for consistent styling.
 *
 * This is kept for backward compatibility - use ConfigRow directly for new code.
 */
export const UnifiedConfigRow: React.FC<UnifiedConfigRowProps> = ({
    label,
    children,
    actions,
    maxWidth = 700,
    labelTooltip,
}) => {
    return (
        <ConfigRow
            label={label}
            content={children}
            actions={actions}
            maxWidth={maxWidth}
            labelTooltip={labelTooltip}
        />
    );
};

export default UnifiedConfigRow;
