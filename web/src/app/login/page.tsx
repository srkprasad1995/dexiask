import { LoginForm } from "@/components/auth/login-form";

export const metadata = {
  title: "Sign in · Dexiask",
};

/**
 * Login page. GitHub token login is the primary method (works with no OAuth app);
 * a GitHub OAuth button is also offered when an OAuth app is configured. See
 * LoginForm for the client logic.
 */
export default function LoginPage() {
  return (
    <main className="flex min-h-full flex-1 items-center justify-center p-6">
      <LoginForm />
    </main>
  );
}
