import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { ChevronRight } from "lucide-react";

interface ResourceNode {
  resource: string; // "" = a whole-root grant (all resources)
  actions: string[] | null; // actions granted at the resource (or root) level
  ids: { id: string; actions: string[] }[]; // specific-resource grants
}

interface RootGroup {
  root: string;
  nodes: ResourceNode[];
}

// buildScopeTree turns the flat scope map (root.userid[.resource[.id]] -> actions)
// into a root -> resource -> {resource-level actions, specific-id grants} hierarchy.
export function buildScopeTree(scopes: Record<string, string[]>): RootGroup[] {
  const roots: Record<string, Record<string, ResourceNode>> = {};
  Object.entries(scopes).forEach(([scope, actions]) => {
    const seg = scope.split(".");
    const root = seg[0];
    const resource = seg.length >= 3 ? seg[2] : "";
    const id = seg.length >= 4 ? seg[3] : null;
    roots[root] ??= {};
    const node = (roots[root][resource] ??= { resource, actions: null, ids: [] });
    if (id) node.ids.push({ id, actions });
    else node.actions = actions;
  });
  return Object.entries(roots)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([root, resMap]) => {
      const nodes = Object.values(resMap).sort((a, b) => a.resource.localeCompare(b.resource));
      nodes.forEach((n) => n.ids.sort((a, b) => a.id.localeCompare(b.id)));
      return { root, nodes };
    });
}

function ActionBadges({ actions }: { actions: string[] }) {
  return (
    <div className="flex gap-1 flex-wrap">
      {actions.map((a) => (
        <Badge key={a} variant="secondary" className="text-xs">
          {a}
        </Badge>
      ))}
    </div>
  );
}

export function ScopeTree({ scopes }: { scopes: Record<string, string[]> }) {
  const tree = buildScopeTree(scopes);
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});

  if (tree.length === 0) {
    return <p className="text-sm text-muted-foreground">No permissions granted.</p>;
  }

  return (
    <div className="space-y-1">
      {tree.map((group) => {
        const open = !collapsed[group.root];
        return (
          <div key={group.root}>
            <button
              type="button"
              onClick={() => setCollapsed((c) => ({ ...c, [group.root]: !c[group.root] }))}
              className="flex items-center gap-2 py-1 w-full text-left"
            >
              <ChevronRight
                className={`w-3.5 h-3.5 text-faint transition-transform ${open ? "rotate-90" : ""}`}
              />
              <span className="font-medium text-foreground">{group.root}</span>
              <span className="text-xs text-faint">
                {group.nodes.length} {group.nodes.length === 1 ? "resource" : "resources"}
              </span>
            </button>

            {open && (
              <div className="ml-[26px] space-y-1.5 pb-1">
                {group.nodes.map((node) => (
                  <div key={node.resource || "__root__"}>
                    <div className="flex items-center gap-2 py-0.5">
                      <span className="text-sm text-muted-foreground">
                        {node.resource || <span className="italic">all resources</span>}
                      </span>
                      {node.actions && <ActionBadges actions={node.actions} />}
                    </div>
                    {node.ids.length > 0 && (
                      <div className="ml-3 mt-0.5 border-l border-border pl-3 space-y-1">
                        {node.ids.map((r) => (
                          <div key={r.id} className="flex items-center gap-2 py-0.5">
                            <span className="font-mono text-xs text-muted-foreground">{r.id}</span>
                            <ActionBadges actions={r.actions} />
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
