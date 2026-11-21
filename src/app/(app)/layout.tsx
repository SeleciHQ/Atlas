import { AppShell } from "@/components/app-shell/app-shell";
import { getSearchIndex, getShellCounts } from "@/lib/queries";
import { getSessionUser } from "@/lib/session";

export default async function AppLayout({ children }: { children: React.ReactNode }) {
  const [session, searchIndex, counts] = await Promise.all([
    getSessionUser(),
    getSearchIndex(),
    getShellCounts(),
  ]);
  return (
    <AppShell
      searchIndex={searchIndex}
      session={{ name: session.name, role: session.role, demoRole: session.demoRole }}
      counts={counts}
    >
      {children}
    </AppShell>
  );
}
