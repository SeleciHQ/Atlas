import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";
import { Slot } from "radix-ui";

const badgeVariants = cva(
  "inline-flex h-[22px] w-fit shrink-0 items-center justify-center gap-1.5 whitespace-nowrap rounded-md border border-transparent px-2 py-0 text-[11.5px] font-medium transition-colors [&>svg]:pointer-events-none [&>svg]:size-3.5",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground [a]:hover:bg-primary/85",
        secondary: "bg-secondary text-muted-foreground [a]:hover:bg-border/60",
        destructive: "bg-danger-bg text-danger",
        success: "bg-success-bg text-success",
        warning: "bg-warning-bg text-warning",
        info: "bg-info-bg text-info",
        brand: "bg-brand-soft text-brand-soft-foreground",
        outline: "border-border text-muted-foreground",
        ghost: "text-muted-foreground",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
);

function Badge({
  className,
  variant = "default",
  asChild = false,
  ...props
}: React.ComponentProps<"span"> &
  VariantProps<typeof badgeVariants> & { asChild?: boolean }) {
  const Comp = asChild ? Slot.Root : "span";
  return (
    <Comp
      data-slot="badge"
      data-variant={variant}
      className={cn(badgeVariants({ variant }), className)}
      {...props}
    />
  );
}

export { Badge, badgeVariants };
