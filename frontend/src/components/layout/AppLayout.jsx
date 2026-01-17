import { Outlet } from "react-router-dom";
import { Sidebar } from "./Sidebar";
import { useHealth } from "@/hooks";
import { useAuth } from "@/contexts/AuthContext";

export function AppLayout() {
  const { user } = useAuth();
  const { health } = useHealth(user, true);

  return (
    <div className="min-h-screen">
      <Sidebar healthOk={health.cluster_ok} />
      <main className="ml-[220px] p-6 min-h-screen">
        <Outlet />
      </main>
    </div>
  );
}
