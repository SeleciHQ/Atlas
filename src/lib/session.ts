import { cookies } from "next/headers";
import { db } from "@/lib/db";

export const ROLE_COOKIE = "atlas-role";

export type DemoRole = "amina" | "jean";

export interface SessionUser {
  id: string;
  name: string;
  email: string;
  role: "FINANCE_OPS" | "CONTROLLER" | "AGENT";
  demoRole: DemoRole;
}

export async function getSessionUser(): Promise<SessionUser> {
  const store = await cookies();
  const demoRole = (store.get(ROLE_COOKIE)?.value as DemoRole) ?? "amina";
  const wanted = demoRole === "jean" ? "CONTROLLER" : "FINANCE_OPS";
  const user = await db.user.findFirst({ where: { role: wanted, id: { not: "usr_agent" } } });
  if (!user) {
    return { id: "", name: demoRole === "jean" ? "Jean Baptiste" : "Amina Diallo", email: "", role: wanted, demoRole };
  }
  return { id: user.id, name: user.name, email: user.email, role: user.role as SessionUser["role"], demoRole };
}
