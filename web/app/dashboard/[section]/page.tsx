import { notFound } from "next/navigation";
import Dashboard from "../workspace";
import { sections, type Section } from "../sections";
export default async function Page({ params }: { params: Promise<{ section: string }> }) {
  const { section } = await params;
  if (!Object.hasOwn(sections, section)) notFound();
  return <Dashboard section={section as Section} />;
}
