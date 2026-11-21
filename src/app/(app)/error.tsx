"use client";

import { useEffect } from "react";
import Link from "next/link";

export default function AppError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <div className="flex flex-col items-center gap-4 py-24 text-center">
      <h2 className="text-[15px] font-semibold text-foreground">Something went wrong</h2>
      <p className="max-w-md text-[13px] text-muted-foreground">
        An unexpected error occurred while loading this page. You can try again, or reset the demo from Settings.
      </p>
      <div className="flex items-center gap-2">
        <button
          onClick={reset}
          className="inline-flex h-9 items-center rounded-md bg-primary px-3.5 text-[13px] font-medium text-primary-foreground transition-colors hover:bg-brand-hover"
        >
          Try again
        </button>
        <Link
          href="/settings"
          className="inline-flex h-9 items-center rounded-md border border-border bg-surface px-3.5 text-[13px] font-medium text-foreground shadow-xs transition-colors hover:bg-surface-subtle"
        >
          Open Settings
        </Link>
      </div>
    </div>
  );
}
