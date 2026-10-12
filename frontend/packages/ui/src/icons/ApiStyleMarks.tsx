import { SvgIcon } from '@mui/material';
import type { SvgIconProps } from '@mui/material';

const commonProps = {
    className: 'api-style-mark-glyph',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: 3.25,
    strokeLinecap: 'square',
    strokeLinejoin: 'round',
} as const;

const frame = <circle className="api-style-mark-frame" cx="12" cy="12" r="11.25" strokeWidth="1.5" />;

export const OpenAIStyleMark = (props: SvgIconProps) => (
    <SvgIcon {...props} viewBox="0 0 24 24">
        {frame}
        <path
            {...commonProps}
            transform="translate(12 12) scale(.68) translate(-12 -12)"
            d="M12 5.1c-3.9 0-6.2 2.7-6.2 6.9s2.3 6.9 6.2 6.9 6.2-2.7 6.2-6.9S15.9 5.1 12 5.1Z"
        />
    </SvgIcon>
);

export const AnthropicStyleMark = (props: SvgIconProps) => (
    <SvgIcon {...props} viewBox="0 0 24 24">
        {frame}
        <path
            {...commonProps}
            transform="translate(12 12) scale(.68) translate(-12 -12)"
            d="M5.8 18.8 12 5.2l6.2 13.6M8.2 13.5h7.6"
        />
    </SvgIcon>
);

export const GoogleStyleMark = (props: SvgIconProps) => (
    <SvgIcon {...props} viewBox="0 0 24 24">
        {frame}
        <path
            {...commonProps}
            transform="translate(12 12) scale(.68) translate(-12 -12)"
            d="M17.6 8.1c-1-1.9-2.8-3-5.3-3-4.1 0-6.7 2.8-6.7 6.9s2.6 6.9 6.7 6.9c2.3 0 4.2-.7 5.5-2v-4.4h-5.3"
        />
    </SvgIcon>
);
