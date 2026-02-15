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
import type { NavItem } from "@/types";

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

export const DEFAULT_NAMESPACE = "default";
export const HIDDEN_NAMESPACE = "hidden";
