import { AppShell } from "@/components/app-shell";
import { AuthProvider, ProtectedProduct } from "@/components/auth-context";

export default function ProductLayout({ children }: { children: React.ReactNode }) {
  return <AuthProvider><ProtectedProduct><AppShell>{children}</AppShell></ProtectedProduct></AuthProvider>;
}
