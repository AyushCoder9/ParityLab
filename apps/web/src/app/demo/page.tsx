import type { Metadata } from "next";
import { SimulationConsole } from "@/components/simulation-console";

export const metadata: Metadata = { title: "Guided simulation" };

export default function DemoPage() {
  return <SimulationConsole />;
}
