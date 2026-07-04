import { AppShell } from "@/components/shell/app-shell";
import { MemoryView } from "@/components/memory/memory-view";

export const metadata = {
  title: "Memory · Dexiask",
};

export default function MemoryPage() {
  return (
    <AppShell title="Memory">
      <MemoryView />
    </AppShell>
  );
}
