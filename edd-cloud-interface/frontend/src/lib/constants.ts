import {
  HardDrive,
  Server,
  MessageSquare,
  Database,
  Activity,
  Settings,
  Box,
  Key,
  KeyRound
} from "lucide-react";
import type { NavItem, TabCopy } from "@/types";

export const NAV_ITEMS: NavItem[] = [
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
  {
    id: "service-accounts",
    label: "Service Accounts",
    icon: KeyRound,
    path: "/service-accounts",
    subItems: [
      { id: "sa-list", label: "Service Accounts", icon: KeyRound, path: "/service-accounts" },
      { id: "sa-tokens", label: "Account Tokens", icon: Key, path: "/service-accounts/tokens" },
    ],
  },
  { id: "health", label: "Health", icon: Activity, path: "/health" },
];

export const ADMIN_NAV_ITEM: NavItem = {
  id: "admin",
  label: "Admin",
  icon: Settings,
  path: "/admin"
};

export const TAB_COPY: Record<string, TabCopy> = {
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
  "service-accounts": {
    eyebrow: "Auth",
    title: "Service Accounts",
    lead: "Create service accounts with scoped permissions and generate tokens for programmatic API access.",
  },
  settings: {
    eyebrow: "Account",
    title: "Settings",
    lead: "Manage security keys, profile, and active sessions.",
  },
} as const;

export const DEFAULT_NAMESPACE = "default";
export const HIDDEN_NAMESPACE = "hidden";
