import type { Metadata } from "next";
import { OnboardingPage } from "@/components/access-pages";
export const metadata: Metadata = { title: "Workspace setup" };
export default function Page() { return <OnboardingPage />; }
