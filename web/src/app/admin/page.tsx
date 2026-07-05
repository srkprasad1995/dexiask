import { AppShell } from "@/components/shell/app-shell";
import { AdminView } from "@/components/admin/admin-view";

export const metadata = {
  title: "Team · Dexiask",
};

export default function AdminPage() {
  return (
    <AppShell title="Team">
      <AdminView />
    </AppShell>
  );
}
