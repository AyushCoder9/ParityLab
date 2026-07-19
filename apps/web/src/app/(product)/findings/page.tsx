import type { Metadata } from "next";
import { FindingsPage } from "@/components/product-pages";
export const metadata: Metadata = { title: "Findings" };
export default function Page() { return <FindingsPage />; }
