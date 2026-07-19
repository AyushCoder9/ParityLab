import type { Metadata } from "next";
import { Dashboard } from "@/components/dashboard";

export const metadata: Metadata = { title: "Integration overview" };

export default function DashboardPage() {
  return <Dashboard />;
}
