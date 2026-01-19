import * as React from "react";
import { Button as MuiButton, ButtonProps as MuiButtonProps } from "@mui/material";

// Mapping of variants to MUI variants
const variantMapping = {
  default: "contained" as const,
  destructive: "contained" as const,
  outline: "outlined" as const,
  secondary: "outlined" as const,
  ghost: "text" as const,
  link: "text" as const,
};

// Mapping of sizes to MUI sizes
const sizeMapping = {
  default: "medium" as const,
  sm: "small" as const,
  lg: "large" as const,
  icon: "medium" as const,
};

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "default" | "destructive" | "outline" | "secondary" | "ghost" | "link";
  size?: "default" | "sm" | "lg" | "icon";
  asChild?: boolean;
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = "default", size = "default", asChild = false, ...props }, ref) => {
    const muiVariant = variantMapping[variant];
    const muiSize = sizeMapping[size];

    const sxProps: MuiButtonProps['sx'] = {
      textTransform: 'none',
      fontWeight: 500,
      gap: '0.5rem',
      ...(variant === 'destructive' && {
        backgroundColor: 'var(--destructive)',
        '&:hover': {
          backgroundColor: 'var(--destructive)',
          opacity: 0.9,
        },
      }),
      ...(variant === 'secondary' && {
        backgroundColor: 'var(--secondary)',
        color: 'var(--secondary-foreground)',
        '&:hover': {
          backgroundColor: 'var(--secondary)',
          opacity: 0.8,
        },
      }),
      ...(variant === 'ghost' && {
        '&:hover': {
          backgroundColor: 'var(--accent)',
          color: 'var(--accent-foreground)',
        },
      }),
      ...(variant === 'link' && {
        textDecoration: 'underline',
        '&:hover': {
          textDecoration: 'underline',
          backgroundColor: 'transparent',
        },
      }),
      ...(size === 'icon' && {
        minWidth: '2.5rem',
        width: '2.5rem',
        height: '2.5rem',
        padding: 0,
      }),
      ...(className && { className }),
    };

    return (
      <MuiButton
        ref={ref}
        variant={muiVariant}
        size={muiSize}
        sx={sxProps}
        {...(props as MuiButtonProps)}
      />
    );
  },
);
Button.displayName = "Button";

export { Button, variantMapping as buttonVariants };
