import { AppShell } from "@/components/shell/app-shell";
import { McpView } from "@/components/mcp/mcp-view";

export const metadata = {
  title: "MCP · Dexiask",
};

export default function McpPage() {
  return (
    <AppShell title="MCP">
      <McpView />
    </AppShell>
  );
}
