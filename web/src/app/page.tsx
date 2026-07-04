import { AppShell } from "@/components/shell/app-shell";
import { Chat } from "@/components/chat/chat";

const SUGGESTIONS = [
  "How is authentication handled in this codebase?",
  "Where is the HTTP server started?",
  "Explain the request flow end to end",
  "Find the database models",
];

export const metadata = {
  title: "Chat · Dexiask",
};

export default function ChatPage() {
  return (
    <AppShell title="Chat">
      <Chat suggestions={SUGGESTIONS} />
    </AppShell>
  );
}
