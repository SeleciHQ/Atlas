export default function AppLoading() {
  return (
    <div className="flex flex-col gap-4" aria-busy="true" aria-live="polite">
      <div className="h-6 w-56 animate-pulse rounded-md bg-surface-subtle" />
      <div className="h-4 w-80 animate-pulse rounded-md bg-surface-subtle" />
      <div className="mt-4 flex flex-col gap-2">
        <div className="h-16 w-full animate-pulse rounded-lg bg-surface-subtle" />
        <div className="h-16 w-full animate-pulse rounded-lg bg-surface-subtle" />
        <div className="h-16 w-full animate-pulse rounded-lg bg-surface-subtle" />
      </div>
      <span className="sr-only">Loading…</span>
    </div>
  );
}
