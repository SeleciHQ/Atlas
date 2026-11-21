"use client";

import * as React from "react";
import { Check, Copy } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * JsonBlock — pretty-prints a JSON-ish value with lightweight token-level
 * syntax highlighting, optional copy-to-clipboard, and monospace font.
 * Designed for small payloads typical of audit logs (up to ~50 lines).
 */
export function JsonBlock({
  value,
  maxHeight = 320,
  className,
  showCopy = true,
}: {
  value: string | object;
  maxHeight?: number;
  className?: string;
  showCopy?: boolean;
}) {
  const [copied, setCopied] = React.useState(false);

  const text = React.useMemo(() => {
    if (typeof value === "string") {
      try {
        return JSON.stringify(JSON.parse(value), null, 2);
      } catch {
        return value;
      }
    }
    try {
      return JSON.stringify(value, null, 2);
    } catch {
      return String(value);
    }
  }, [value]);

  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // ignore clipboard errors (e.g. insecure contexts)
    }
  }

  return (
    <div className={cn("relative rounded-md border border-border bg-surface", className)}>
      {showCopy && (
        <button
          type="button"
          onClick={copy}
          aria-label={copied ? "Copied" : "Copy payload"}
          className="absolute right-1.5 top-1.5 flex h-6 w-6 items-center justify-center rounded text-muted-foreground hover:bg-surface-subtle hover:text-foreground"
        >
          {copied ? <Check className="h-3.5 w-3.5 text-success" /> : <Copy className="h-3.5 w-3.5" />}
        </button>
      )}
      <pre
        className="overflow-auto scroll-thin px-3 py-2.5 font-mono text-[11.5px] leading-[1.6]"
        style={{ maxHeight }}
      >
        <span dangerouslySetInnerHTML={{ __html: highlight(text) }} />
      </pre>
    </div>
  );
}

// Tiny JSON aware highlighter — escapes HTML, then wraps token classes.
function highlight(input: string): string {
  const escaped = escapeHtml(input);
  // order matters: strings, numbers, booleans/null
  return escaped
    .replace(/&quot;([^&]*?)&quot;(\s*):/g, '<span class="json-key">"$1"</span>$2:')
    .replace(/: &quot;(.*?)&quot;/g, ': <span class="json-str">"$1"</span>')
    .replace(/: (-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g, ': <span class="json-num">$1</span>')
    .replace(/: (true|false|null)/g, ': <span class="json-bool">$1</span>');
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}
