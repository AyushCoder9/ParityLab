import type { Metadata } from "next";
import { RunsPage } from "@/components/product-pages";
export const metadata: Metadata = { title: "Verification runs" };
export default function Page() { return <RunsPage />; }
