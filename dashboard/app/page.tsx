import { redirect } from "next/navigation";
import { getServerSession } from "next-auth";
import { authOptions } from "./api/auth/[...nextauth]/options";

export default async function Home() {
  const s = await getServerSession(authOptions);
  if (s) redirect("/projects");
  redirect("/login");
}
