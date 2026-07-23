import type { Metadata } from "next";
import { OnboardingPage } from "@/components/access-pages";
import { AuthProvider, ProtectedProduct } from "@/components/auth-context";
export const metadata: Metadata = { title: "Workspace setup" };
export default function Page() { return <AuthProvider><ProtectedProduct><OnboardingPage /></ProtectedProduct></AuthProvider>; }
