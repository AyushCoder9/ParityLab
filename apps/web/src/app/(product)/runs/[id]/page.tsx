import type { Metadata } from "next";
import { RunDetailPage } from "@/components/product-pages";
export const metadata: Metadata = { title: "Run evidence" };
export default async function Page({ params }: { params: Promise<{ id: string }> }) { const { id } = await params; return <RunDetailPage id={id} />; }
