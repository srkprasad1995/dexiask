import { AppShell } from "@/components/shell/app-shell";
import { IndexerView } from "@/components/indexer/indexer-view";

export const metadata = {
  title: "Indexer · Dexiask",
};

export default function IndexerPage() {
  return (
    <AppShell title="Indexer">
      <IndexerView />
    </AppShell>
  );
}
