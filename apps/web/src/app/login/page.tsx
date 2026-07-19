import type { Metadata } from "next";
import { LoginPage } from "@/components/access-pages";
export const metadata: Metadata = { title: "Local access" };
export default function Page() { return <LoginPage />; }
