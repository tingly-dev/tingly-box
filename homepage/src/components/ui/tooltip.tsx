import * as React from "react";
import { Tooltip as MuiTooltip, tooltipClasses } from "@mui/material";

// MUI Tooltip wrapper for consistency with existing API
const TooltipProvider = ({ children }: { children: React.ReactNode }) => {
  // MUI doesn't require a provider like Radix UI, but we keep this for backward compatibility
  return <>{children}</>;
};

const Tooltip = ({ children, ...props }: React.ComponentProps<typeof MuiTooltip>) => {
  return (
    <MuiTooltip
      {...props}
      slotProps={{
        tooltip: {
          sx: {
            backgroundColor: 'var(--popover)',
            color: 'var(--popover-foreground)',
            border: '1px solid var(--border)',
            fontSize: '0.875rem',
            padding: '0.375rem 0.75rem',
            borderRadius: '0.375rem',
            maxWidth: '300px',
            fontWeight: 400,
            boxShadow: 'var(--shadow)',
            [`&.${tooltipClasses.arrow}`]: {
              color: 'var(--popover)',
              '&::before': {
                border: '1px solid var(--border)',
              },
            },
          },
        },
        arrow: {
          sx: {
            color: 'var(--popover)',
          },
        },
      }}
      arrow
    >
      {children}
    </MuiTooltip>
  );
};

const TooltipTrigger = React.forwardRef<
  React.ElementRef<"button">,
  React.ComponentPropsWithoutRef<"button">
>(({ className, ...props }, ref) => (
  <button
    className={className}
    ref={ref}
    {...props}
  />
));
TooltipTrigger.displayName = "TooltipTrigger";

const TooltipContent = React.forwardRef<
  React.ElementRef<"div">,
  React.ComponentPropsWithoutRef<"div">
>(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={className}
    {...props}
  />
));
TooltipContent.displayName = "TooltipContent";

// Export MUI tooltip components with original names for backward compatibility
export { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider };
