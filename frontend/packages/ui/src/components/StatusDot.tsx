import { Box } from '@mui/material';
import type { SxProps, Theme } from '@mui/material/styles';
import { getStatusColor, type StatusTone } from '../theme/status';

interface StatusDotProps {
  tone: StatusTone;
  /** Diameter in px. */
  size?: number;
  sx?: SxProps<Theme>;
}

/**
 * A status marker: colour sits on this dot, the text next to it stays
 * neutral (`.design/visual-system.md` §2.4 rule 3). Decorative — the text
 * beside it must carry the meaning on its own.
 */
export default function StatusDot({ tone, size = 6, sx }: StatusDotProps) {
  return (
    <Box
      component="span"
      aria-hidden="true"
      sx={[
        (theme) => ({
          display: 'inline-block',
          flexShrink: 0,
          width: size,
          height: size,
          borderRadius: '50%',
          bgcolor: getStatusColor(theme, tone),
        }),
        ...(Array.isArray(sx) ? sx : [sx]),
      ]}
    />
  );
}
