import { NavLink as RouterNavLink, NavLinkProps } from "react-router-dom";
import { forwardRef } from "react";
import { Button, ButtonProps } from "@mui/material";

interface NavLinkCompatProps extends Omit<NavLinkProps, "className" | "children"> {
  className?: string;
  activeClassName?: string;
  pendingClassName?: string;
  variant?: ButtonProps['variant'];
  size?: ButtonProps['size'];
  startIcon?: React.ReactNode;
  endIcon?: React.ReactNode;
  sx?: ButtonProps['sx'];
  children: React.ReactNode | ((props: { isActive: boolean; isPending: boolean }) => React.ReactNode);
}

const NavLink = forwardRef<HTMLAnchorElement, NavLinkCompatProps>(
  ({
    className,
    activeClassName,
    pendingClassName,
    to,
    variant = "text",
    size = "medium",
    startIcon,
    endIcon,
    sx,
    children,
    ...props
  }, ref) => {
    return (
      <RouterNavLink
        ref={ref}
        to={to}
        className={({ isActive, isPending }) => {
          const activeClass = isActive && activeClassName ? activeClassName : '';
          const pendingClass = isPending && pendingClassName ? pendingClassName : '';
          return `${className || ''} ${activeClass} ${pendingClass}`.trim();
        }}
        style={{ textDecoration: 'none' }}
        {...props}
      >
        {({ isActive, isPending }) => {
          const childContent = typeof children === 'function' ? children({ isActive, isPending }) : children;

          return (
            <Button
              variant={variant}
              size={size}
              startIcon={startIcon}
              endIcon={endIcon}
              sx={{
                '&:hover': {
                  textDecoration: 'none',
                },
                ...(isActive && activeClassName ? {
                  backgroundColor: 'var(--accent)',
                  color: 'var(--accent-foreground)',
                } : {}),
                ...(isPending && pendingClassName ? {
                  opacity: 0.7,
                } : {}),
                ...sx,
              }}
            >
              {childContent}
            </Button>
          );
        }}
      </RouterNavLink>
    );
  },
);

NavLink.displayName = "NavLink";

export { NavLink };
