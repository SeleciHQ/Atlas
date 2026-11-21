import Link from "next/link";

export default function NotFound() {
  return (
    <div className="flex min-h-dvh flex-col items-center justify-center gap-4 px-6 text-center">
      <p className="label-caps">404</p>
      <h1 className="text-[15px] font-semibold text-foreground">Record not found</h1>
      <p className="max-w-md text-[13px] text-muted-foreground">
        The case, invoice, supplier, or approval you are looking for does not exist. It may have been removed when the
        demo was reset.
      </p>
      <Link
        href="/work"
        className="inline-flex h-9 items-center rounded-md bg-primary px-3.5 text-[13px] font-medium text-primary-foreground transition-colors hover:bg-brand-hover"
      >
        Back to Work
      </Link>
    </div>
  );
}
