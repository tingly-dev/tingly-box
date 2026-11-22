import { Switch, type SwitchProps } from '@mui/material';

export type TriState = 'on' | 'off' | 'mixed';

/** Collapse a list of on/off values into one group state. Empty is off. */
export const triStateOf = (values: boolean[]): TriState => {
  const on = values.filter(Boolean).length;
  return on === 0 ? 'off' : on === values.length ? 'on' : 'mixed';
};

interface TriStateSwitchProps extends Omit<SwitchProps, 'checked' | 'onChange'> {
  state: TriState;
  /** A mixed group turns fully on when clicked, the same as a tri-state checkbox. */
  onChange: (on: boolean) => void;
}

/**
 * A switch for a group of switches: on when all are on, off when none are, and
 * a half-way thumb when only some are. It writes one value for the whole group.
 * The mixed state is visual; callers say "N of M on" beside it in text.
 */
export default function TriStateSwitch({ state, onChange, sx, slotProps, ...rest }: TriStateSwitchProps) {
  const mixed = state === 'mixed';
  return (
    <Switch
      {...rest}
      checked={state === 'on'}
      onChange={(e) => onChange(e.target.checked)}
      // The state is on the root so it can be styled and asserted on.
      slotProps={{ ...slotProps, root: { ...slotProps?.root, 'data-state': state } as object }}
      sx={[
        // Mixed: the thumb stops half-way, hollow, over a pale track, so it
        // reads as neither on nor off at a glance.
        mixed && {
          '& .MuiSwitch-switchBase': {
            transform: rest.size === 'small' ? 'translateX(8px)' : 'translateX(10px)',
          },
          '& .MuiSwitch-thumb': {
            bgcolor: 'background.paper',
            boxShadow: (theme) => `inset 0 0 0 4px ${theme.palette.primary.main}`,
          },
          '& .MuiSwitch-switchBase + .MuiSwitch-track': { bgcolor: 'primary.main', opacity: 0.3 },
        },
        ...(Array.isArray(sx) ? sx : [sx]),
      ]}
    />
  );
}
