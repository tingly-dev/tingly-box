import { forwardRef } from 'react';
import { SvgIcon } from '@mui/material';
import type { SvgIconProps } from '@mui/material';
import type { SvgIconComponent } from '@mui/icons-material';
import type { Icon as TablerIcon } from '@tabler/icons-react';

/**
 * Wraps a `@tabler/icons-react` icon so it behaves like an `@mui/icons-material`
 * icon: it inherits MUI's `fontSize` sizing and the theme `color` semantics.
 *
 * Tabler icons are outline (stroke) based and default to `fill="none"`, while
 * MUI's SvgIcon root forces `fill: currentColor`. We override `fill` back to
 * `none` so the outline style is preserved, and let `stroke="currentColor"`
 * follow the MUI `color` prop / surrounding `color` CSS.
 */
export function tablerMui(Icon: TablerIcon, defaultStrokeWidth = 1.75) {
    const Wrapped = forwardRef<SVGSVGElement, SvgIconProps>(function TablerMuiIcon(props, ref) {
        const { sx, ...rest } = props;
        return (
            <SvgIcon
                ref={ref}
                component={Icon}
                inheritViewBox
                stroke={defaultStrokeWidth as unknown as string}
                sx={[{ fill: 'none' }, ...(Array.isArray(sx) ? sx : [sx])]}
                {...rest}
            />
        );
    });
    Wrapped.displayName = `TablerMui(${Icon.displayName ?? 'Icon'})`;
    // Typed as MUI's SvgIconComponent so these are drop-in compatible anywhere
    // an `@mui/icons-material` icon is expected.
    return Wrapped as unknown as SvgIconComponent;
}
