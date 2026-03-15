import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { PageHeader } from "@/components/ui/page-header";
import { Breadcrumb } from "@/components/ui/breadcrumb";
import { RepoList, RepoDetail } from "@/components/registry";
import { useRegistry } from "@/hooks/useRegistry";
import { useAuth } from "@/contexts/AuthContext";

const REGISTRY_BASE = "/storage/registry";

function parseRepoFromPath(pathname: string): string | null {
  // pathname: /storage/registry/test/echo -> "test/echo"
  // pathname: /storage/registry -> null
  const prefix = REGISTRY_BASE + "/";
  if (pathname.startsWith(prefix)) {
    const rest = pathname.slice(prefix.length);
    if (rest.length > 0) return rest;
  }
  return null;
}

export function RegistryPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const { userId } = useAuth();

  const repoName = parseRepoFromPath(location.pathname);

  const { myRepos, publicRepos, loading, error, loadRepos, loadTags, setVisibility, deleteTag } =
    useRegistry(userId ?? undefined);

  const [activeTab, setActiveTab] = useState<"mine" | "public">("mine");

  const handleSelectRepo = (name: string) => {
    navigate(`${REGISTRY_BASE}/${name}`);
  };

  const handleBack = () => {
    navigate(REGISTRY_BASE);
  };

  // Find the selected repo info for visibility/ownerId
  const allRepos = [...myRepos, ...publicRepos];
  const selectedRepo = repoName ? allRepos.find((r) => r.name === repoName) : null;

  // Detail view
  if (repoName) {
    return (
      <div>
        <Breadcrumb
          items={[
            { label: "Storage", href: "/storage" },
            { label: "Registry", href: REGISTRY_BASE },
            { label: repoName },
          ]}
        />
        <PageHeader
          title={repoName}
          description="Container image repository."
        />
        <RepoDetail
          repoName={repoName}
          ownerId={selectedRepo?.owner_id ?? ""}
          visibility={selectedRepo?.visibility ?? 0}
          currentUserId={userId ?? undefined}
          onBack={handleBack}
          onLoadTags={loadTags}
          onDeleteTag={async (name, tag) => {
            await deleteTag(name, tag);
          }}
          onSetVisibility={async (name, vis) => {
            await setVisibility(name, vis);
          }}
        />
      </div>
    );
  }

  // List view
  return (
    <div>
      <Breadcrumb
        items={[
          { label: "Storage", href: "/storage" },
          { label: "Registry" },
        ]}
      />
      <PageHeader
        title="Registry"
        description="Manage container image repositories."
      />

      {error && (
        <div className="bg-destructive/10 border border-destructive/20 rounded-lg px-4 py-3 mb-4">
          <p className="text-sm text-destructive">{error}</p>
        </div>
      )}

      {/* Tabs */}
      <div className="flex gap-1 mb-4 border-b border-border">
        <button
          className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px ${
            activeTab === "mine"
              ? "border-primary text-foreground"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
          onClick={() => setActiveTab("mine")}
        >
          My Repositories
        </button>
        <button
          className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px ${
            activeTab === "public"
              ? "border-primary text-foreground"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
          onClick={() => setActiveTab("public")}
        >
          Public
        </button>
      </div>

      <div className="bg-card border border-border rounded-lg">
        <div className="px-5 py-4 border-b border-border">
          <h2 className="text-sm font-semibold">
            {activeTab === "mine" ? "My Repositories" : "Public Repositories"}
          </h2>
        </div>
        {loading ? (
          <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
            Loading repositories...
          </div>
        ) : (
          <RepoList
            repos={activeTab === "mine" ? myRepos : publicRepos}
            onSelect={handleSelectRepo}
          />
        )}
      </div>
    </div>
  );
}
