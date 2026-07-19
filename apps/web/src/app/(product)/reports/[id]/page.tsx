import type { Metadata } from "next";
import { ReportPage } from "@/components/product-pages";
export const metadata: Metadata = { title: "Evidence report" };
export default async function Page({ params }: { params: Promise<{ id: string }> }) { const { id } = await params; return <ReportPage id={id} />; }
