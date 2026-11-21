import { NextResponse } from "next/server";
import { resetAndSeed } from "@/lib/demo-data";

export async function POST() {
  try {
    await resetAndSeed();
    return NextResponse.json({ ok: true, message: "Demo data reset to the original seed." });
  } catch (error) {
    return NextResponse.json(
      { ok: false, message: "Failed to reset demo data.", detail: String(error) },
      { status: 500 }
    );
  }
}
