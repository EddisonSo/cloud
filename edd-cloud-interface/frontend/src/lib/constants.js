import {
  HardDrive,
  Server,
  MessageSquare,
  Database,
  Activity,
  Settings,
  Box,
  Key
} from "lucide-react";

export const NAV_ITEMS = [
  {
    id: "compute",
    label: "Compute",
    icon: Server,
    path: "/compute",
    subItems: [
      { id: "containers", label: "Containers", icon: Box, path: "/compute/containers" },
      { id: "ssh-keys", label: "SSH Keys", icon: Key, path: "/compute/ssh-keys" },
    ],
  },
  { id: "storage", label: "Storage", icon: HardDrive, path: "/storage" },
  { id: "message-queue", label: "Message Queue", icon: MessageSquare, path: "/message-queue" },
  { id: "datastore", label: "Datastore", icon: Database, path: "/datastore" },
  { id: "health", label: "Health", icon: Activity, path: "/health" },
];

export const ADMIN_NAV_ITEM = {
  id: "admin",
  label: "Admin",
  icon: Settings,
  path: "/admin"
};

export const TAB_COPY = {
  storage: {
    eyebrow: "Cloud Storage",
    title: "Simple File Share",
    lead: "Manage shared assets with clear status, fast uploads, and controlled access.",
  },
  compute: {
    eyebrow: "Compute Services",
    title: "Stateful Containers",
    lead: "Stateful containers with persistent storage and dedicated IPs.",
  },
  "message-queue": {
    eyebrow: "Messaging",
    title: "Message Queue",
    lead: "Queue and stream services are not available yet, but the surface is ready.",
  },
  datastore: {
    eyebrow: "Data Systems",
    title: "Datastore",
    lead: "Datastore provisioning is coming soon with managed database workflows.",
  },
  health: {
    eyebrow: "Operations",
    title: "Health Monitor",
    lead: "Live telemetry for master connectivity and chunkserver status.",
  },
  admin: {
    eyebrow: "Administration",
    title: "Admin Panel",
    lead: "View all files and containers across the system.",
  },
};

export const DEFAULT_NAMESPACE = "default";
export const HIDDEN_NAMESPACE = "hidden";
