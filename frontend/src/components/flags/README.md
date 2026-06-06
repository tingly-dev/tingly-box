# Flag Components

This directory contains reusable UI components for controlling various scenario flags.

## Components

### SessionAffinityControl
Control for session affinity TTL with preset options and custom value input.

**Props:**
- `value: number` - Current TTL value in seconds (0 = disabled)
- `onChange: (value: number) => void` - Callback when value changes
- `disabled?: boolean` - Disable the control

## Usage

```tsx
import { SessionAffinityControl } from '@/components/flags';

<SessionAffinityControl
    value={sessionAffinity}
    onChange={handleSessionAffinityChange}
    disabled={updating}
/>
```

## Future Components

Consider adding more flag control components here:
- `ThinkingControl.tsx` - Thinking effort level control
- `VisionProxyControl.tsx` - Vision proxy service picker
- `RecordingControl.tsx` - Recording mode control
- `PluginFlagControl.tsx` - Generic boolean flag toggle
