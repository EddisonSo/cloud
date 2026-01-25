import { useCallback, useMemo, useEffect } from "react";
import {
  ReactFlow,
  Controls,
  Background,
  Handle,
  Position,
  useNodesState,
  useEdgesState,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { useServiceGraph } from "@/hooks";
import { Skeleton } from "@/components/ui/skeleton";

const edgeColors = {
  http: "#3b82f6",   // blue
  grpc: "#a855f7",   // purple
  db: "#f97316",     // orange
  nats: "#22c55e",   // green
};

const nodeTypeColors = {
  service: "#3b82f6",
  database: "#f97316",
  messaging: "#22c55e",
  storage: "#8b5cf6",
};

function ServiceNode({ data }) {
  const typeColor = nodeTypeColors[data.type] || "#6b7280";
  const healthColor = data.health === "healthy" ? "#22c55e" : data.health === "degraded" ? "#f59e0b" : "#6b7280";

  return (
    <div
      className="px-4 py-3 rounded-lg border-2 bg-background shadow-sm min-w-[120px] relative"
      style={{ borderColor: typeColor }}
    >
      <Handle type="target" position={Position.Left} className="!bg-muted-foreground !w-2 !h-2" />
      <div className="flex items-center gap-2">
        <div
          className="w-2 h-2 rounded-full"
          style={{ backgroundColor: healthColor }}
        />
        <span className="font-medium text-sm">{data.label}</span>
      </div>
      <div className="text-xs text-muted-foreground mt-1 capitalize">
        {data.type}
      </div>
      <Handle type="source" position={Position.Right} className="!bg-muted-foreground !w-2 !h-2" />
    </div>
  );
}

const nodeTypes = {
  service: ServiceNode,
};

function layoutNodes(nodes, edges) {
  // Simple hierarchical layout
  const nodeById = new Map(nodes.map((n) => [n.id, n]));
  const incoming = new Map();
  const outgoing = new Map();

  // Build adjacency maps
  for (const edge of edges) {
    if (!outgoing.has(edge.source)) outgoing.set(edge.source, []);
    outgoing.get(edge.source).push(edge.target);
    if (!incoming.has(edge.target)) incoming.set(edge.target, []);
    incoming.get(edge.target).push(edge.source);
  }

  // Find root nodes (no incoming edges)
  const roots = nodes.filter((n) => !incoming.has(n.id) || incoming.get(n.id).length === 0);

  // BFS to assign levels
  const levels = new Map();
  const queue = roots.map((n) => ({ id: n.id, level: 0 }));
  const visited = new Set();

  while (queue.length > 0) {
    const { id, level } = queue.shift();
    if (visited.has(id)) continue;
    visited.add(id);
    levels.set(id, level);

    for (const targetId of outgoing.get(id) || []) {
      if (!visited.has(targetId)) {
        queue.push({ id: targetId, level: level + 1 });
      }
    }
  }

  // Group by level
  const levelGroups = new Map();
  for (const [id, level] of levels) {
    if (!levelGroups.has(level)) levelGroups.set(level, []);
    levelGroups.get(level).push(id);
  }

  // Add unvisited nodes to last level
  for (const node of nodes) {
    if (!visited.has(node.id)) {
      const maxLevel = Math.max(...Array.from(levelGroups.keys()), 0);
      if (!levelGroups.has(maxLevel + 1)) levelGroups.set(maxLevel + 1, []);
      levelGroups.get(maxLevel + 1).push(node.id);
    }
  }

  // Position nodes
  const xSpacing = 200;
  const ySpacing = 120;
  const positioned = [];

  for (const [level, nodeIds] of Array.from(levelGroups.entries()).sort((a, b) => a[0] - b[0])) {
    const yOffset = -(nodeIds.length - 1) * ySpacing / 2;
    nodeIds.forEach((id, index) => {
      const node = nodeById.get(id);
      if (node) {
        positioned.push({
          ...node,
          position: {
            x: level * xSpacing,
            y: yOffset + index * ySpacing,
          },
          type: "service",
        });
      }
    });
  }

  return positioned;
}

export function ServiceDependencyGraph({ healthData }) {
  const { nodes: rawNodes, edges: rawEdges, loading, error, refetch } = useServiceGraph(healthData);

  const initialNodes = useMemo(() => {
    if (!rawNodes || rawNodes.length === 0) return [];

    const nodesWithData = rawNodes.map((node) => ({
      id: node.id,
      data: {
        label: node.label,
        type: node.type,
        health: node.health || "unknown",
      },
    }));

    return layoutNodes(nodesWithData, rawEdges || []);
  }, [rawNodes, rawEdges]);

  const initialEdges = useMemo(() => {
    if (!rawEdges) return [];

    return rawEdges.map((edge) => ({
      id: edge.id,
      source: edge.source,
      target: edge.target,
      style: {
        stroke: edgeColors[edge.type] || "#6b7280",
        strokeWidth: 2,
      },
      animated: edge.type === "nats",
      label: edge.label,
    }));
  }, [rawEdges]);

  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

  // Update when data changes
  useEffect(() => {
    if (initialNodes.length > 0) {
      setNodes(initialNodes);
    }
    if (initialEdges.length > 0) {
      setEdges(initialEdges);
    }
  }, [initialNodes, initialEdges, setNodes, setEdges]);

  if (loading && rawNodes.length === 0) {
    return (
      <div className="h-[700px] flex items-center justify-center">
        <Skeleton className="w-full h-full" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="h-[700px] flex items-center justify-center flex-col gap-4">
        <p className="text-destructive">{error}</p>
        <button
          onClick={refetch}
          className="text-sm text-primary hover:underline"
        >
          Retry
        </button>
      </div>
    );
  }

  if (nodes.length === 0) {
    return (
      <div className="h-[700px] flex items-center justify-center text-muted-foreground">
        No service dependencies defined
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold">Service Dependencies</h3>
        <button
          onClick={refetch}
          className="text-sm text-muted-foreground hover:text-foreground transition-colors"
        >
          Refresh
        </button>
      </div>

      <div className="h-[700px] border rounded-lg bg-background">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          nodeTypes={nodeTypes}
          fitView
          attributionPosition="bottom-left"
        >
          <Controls />
          <Background />
        </ReactFlow>
      </div>

      <div className="flex flex-wrap gap-4 text-xs">
        <div className="flex items-center gap-2">
          <span className="font-medium">Edge Types:</span>
        </div>
        {Object.entries(edgeColors).map(([type, color]) => (
          <div key={type} className="flex items-center gap-1.5">
            <div className="w-4 h-0.5" style={{ backgroundColor: color }} />
            <span className="capitalize">{type}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
