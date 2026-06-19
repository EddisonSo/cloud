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

  const { repos, loading, error, loadRepos, loadTags, deleteTag } = useRegistry();

  const handleSelectRepo = (name: string) => {
    navigate(`${REGISTRY_BASE}/${name}`);
  };

  const handleBack = () => {
    navigate(REGISTRY_BASE);
  };

  // Detail view
  if (repoName) {
    const selectedRepo = repos.find((r) => r.name === repoName);
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
          currentUserId={userId ?? undefined}
          onBack={handleBack}
          onLoadTags={loadTags}
          onDeleteTag={async (name, tag) => {
            await deleteTag(name, tag);
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
        <div className="border border-destructive/30 px-4 py-3 mb-4">
          <p className="text-sm text-destructive">{error}</p>
        </div>
      )}

      <div className="bg-card border border-border">
        <div className="px-5 py-4 border-b border-border">
          <h2 className="font-mono text-[10.5px] font-medium uppercase tracking-[0.2em] text-muted-foreground">
            My Repositories
          </h2>
        </div>
        {loading ? (
          <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
            Loading repositories...
          </div>
        ) : (
          <RepoList
            repos={repos}
            onSelect={handleSelectRepo}
          />
        )}
      </div>
    </div>
  );
}
